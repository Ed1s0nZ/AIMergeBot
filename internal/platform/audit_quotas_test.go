package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func bindTestQuotas(s *Store, q AuditQuotas) {
	defaultAuditQuotas(&q)
	svc := &SettingsService{}
	svc.current.Store(&Settings{AuditQuotas: q})
	s.BindQuotaSettings(svc)
}
func quotaSnapshot(project, mr int) Snapshot {
	return Snapshot{ProjectID: project, SourceProjectID: project, MRIID: mr, BaseSHA: "base", HeadSHA: fmt.Sprintf("head-%d", mr)}
}
func enqueueQuota(t *testing.T, s *Store, project, mr int, actor int64) int64 {
	t.Helper()
	id, created, err := s.Enqueue(context.Background(), quotaSnapshot(project, mr), actor, false)
	if err != nil || !created {
		t.Fatal("enqueue", id, created, err)
	}
	return id
}
func TestConcurrentQuotaAdmissionAndDuplicateReuse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bindTestQuotas(s, AuditQuotas{OutstandingGlobal: 7})
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for n := 1; n <= 40; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, created, err := s.Enqueue(ctx, quotaSnapshot(n, n), 0, false)
			var q *QuotaError
			if err != nil {
				if !errors.As(err, &q) || q.Scope != "outstanding_global" {
					t.Error(err)
				}
				return
			}
			if created {
				admitted.Add(1)
			}
		}(n)
	}
	wg.Wait()
	if admitted.Load() != 7 {
		t.Fatal("capacity not atomic", admitted.Load())
	}
	var id int64
	var project, mr int
	if err := s.DB.QueryRow(`SELECT id,project_id,mr_iid FROM platform_runs LIMIT 1`).Scan(&id, &project, &mr); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		reused, created, err := s.Enqueue(ctx, quotaSnapshot(project, mr), 0, force)
		if err != nil || created || reused != id {
			t.Fatal("duplicate rejected at full capacity", err)
		}
	}
	if err := s.Cancel(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	enqueueQuota(t, s, 99, 99, 0)
}
func TestQuotaBucketsApplyToAdminAndSystemRequests(t *testing.T) {
	for _, scope := range []string{"outstanding_project", "outstanding_user"} {
		t.Run(scope, func(t *testing.T) {
			s := testStore(t)
			q := AuditQuotas{OutstandingProject: 2, OutstandingUser: 2}
			bindTestQuotas(s, q)
			ctx := context.Background()
			enqueueQuota(t, s, 1, 1, 1)
			enqueueQuota(t, s, 1, 2, 1)
			project := 1
			if scope == "outstanding_user" {
				project = 2
			}
			_, _, err := s.Enqueue(ctx, quotaSnapshot(project, 3), 1, false)
			var quota *QuotaError
			if !errors.As(err, &quota) || quota.Scope != scope {
				t.Fatal(err)
			}
			if scope == "outstanding_project" {
				_, _, err = s.Enqueue(ctx, quotaSnapshot(1, 4), 0, false)
				if !errors.As(err, &quota) || quota.Scope != scope {
					t.Fatal("system bypassed project capacity", err)
				}
			} else {
				enqueueQuota(t, s, 2, 4, 0)
			}
		})
	}
}
func TestClaimSkipsBlockedUsersAndProjectsWithoutStarvation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bindTestQuotas(s, AuditQuotas{RunningProject: 1, RunningUser: 1})
	first := enqueueQuota(t, s, 1, 1, 1)
	blockedProject := enqueueQuota(t, s, 1, 2, 2)
	blockedUser := enqueueQuota(t, s, 2, 3, 1)
	other := enqueueQuota(t, s, 2, 4, 2)
	if got, err := s.Claim(ctx); err != nil || got != first {
		t.Fatal(got, err)
	}
	if got, err := s.Claim(ctx); err != nil || got != other {
		t.Fatal("blocked FIFO head starved another user/project", got, err)
	}
	for _, id := range []int64{blockedProject, blockedUser} {
		run, _ := s.Run(ctx, id)
		wait, err := s.QueueWaitFor(ctx, run)
		if err != nil || wait == nil || wait.Reason != "project_running" {
			t.Fatal(wait, err)
		}
	}
	if _, err := s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("running quota bypass", err)
	}
	if err := s.Finish(ctx, first, "failed", "", AuditResult{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("free project bypassed occupied user slot", err)
	}
	if err := s.Cancel(ctx, other, 2); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Claim(ctx); err != nil || got != blockedProject {
		t.Fatal(got, err)
	}
	if got, err := s.Claim(ctx); err != nil || got != blockedUser {
		t.Fatal("cancel did not release execution slot", got, err)
	}
}
func TestConcurrentClaimsRespectRunningProjectLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bindTestQuotas(s, AuditQuotas{RunningProject: 1})
	for n := 1; n <= 10; n++ {
		enqueueQuota(t, s, 1, n, 0)
	}
	var claimed atomic.Int64
	var wg sync.WaitGroup
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Claim(ctx); err == nil {
				claimed.Add(1)
			} else if !errors.Is(err, sql.ErrNoRows) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatal("parallel claim exceeded quota", claimed.Load())
	}
}
func TestRollingQuotaCountsStartedAttemptsAndExpires(t *testing.T) {
	for _, scope := range []string{"global_daily", "project_daily", "user_daily"} {
		t.Run(scope, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			q := AuditQuotas{}
			switch scope {
			case "global_daily":
				q.DailyGlobal = 1
			case "project_daily":
				q.DailyProject = 1
			case "user_daily":
				q.DailyUser = 1
			}
			bindTestQuotas(s, q)
			first := enqueueQuota(t, s, 1, 1, 1)
			waiting := enqueueQuota(t, s, 1, 2, 1)
			if got, err := s.Claim(ctx); err != nil || got != first {
				t.Fatal(got, err)
			}
			if err := s.Finish(ctx, first, "failed", "", AuditResult{}, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("failed start refunded quota", err)
			}
			run, _ := s.Run(ctx, waiting)
			wait, err := s.QueueWaitFor(ctx, run)
			if err != nil || wait == nil || wait.Reason != scope {
				t.Fatal(wait, err)
			}
			eligible, err := time.Parse(time.RFC3339Nano, wait.EligibleAt)
			if err != nil || time.Until(eligible) < 23*time.Hour {
				t.Fatal("missing rolling expiry", wait, err)
			}
			other := enqueueQuota(t, s, 2, 3, 2)
			got, err := s.Claim(ctx)
			if scope == "global_daily" {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("global bypass", got, err)
				}
			} else {
				if err != nil || got != other {
					t.Fatal("quota starved other bucket", got, err)
				}
			}
			if _, err = s.DB.Exec(`UPDATE platform_runs SET started_at=? WHERE id=?`, time.Now().UTC().Add(-25*time.Hour).Format(time.RFC3339Nano), first); err != nil {
				t.Fatal(err)
			}
			if got, err = s.Claim(ctx); err != nil || got != waiting {
				t.Fatal("rolling quota did not expire", got, err)
			}
		})
	}
}
func TestRetryQuotaSurvivesRestartAndPreservesReservedCapacity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "quota.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	q := AuditQuotas{OutstandingGlobal: 1, DailyProject: 1}
	bindTestQuotas(s, q)
	parent := enqueueQuota(t, s, 1, 1, 0)
	s.Claim(ctx)
	child, err := s.FailAndRetry(ctx, parent, "temporary", AuditResult{Summary: "checkpoint"}, nil, 0)
	if err != nil || child == 0 {
		t.Fatal(child, err)
	}
	if _, _, err = s.Enqueue(ctx, quotaSnapshot(2, 2), 0, false); err == nil {
		t.Fatal("retry did not retain capacity")
	}
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("retry bypassed rolling quota", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bindTestQuotas(s, q)
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, child)
	if err != nil || run.Status != "pending" || run.RetryAttempt != 1 {
		t.Fatal(run, err)
	}
	wait, err := s.QueueWaitFor(ctx, run)
	if err != nil || wait == nil || wait.Reason != "project_daily" {
		t.Fatal(wait, err)
	}
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("restart reset rolling quota", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET started_at=? WHERE id=?`, time.Now().UTC().Add(-25*time.Hour).Format(time.RFC3339Nano), parent); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Claim(ctx); err != nil || got != child {
		t.Fatal(got, err)
	}
	if err = s.Cancel(ctx, child, 0); err != nil {
		t.Fatal(err)
	}
	enqueueQuota(t, s, 1, 3, 0)
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("cancellation refunded started retry", err)
	}
	original, _ := s.Run(ctx, parent)
	if original.Result.Summary != "checkpoint" {
		t.Fatal("quota lost evidence")
	}
}
func TestQuotaSettingsPersistApplyLiveAndRetainOmittedFields(t *testing.T) {
	svc, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	s.BindQuotaSettings(svc)
	ctx := context.Background()
	cfg := svc.Snapshot()
	cfg.AuditQuotas.OutstandingGlobal = 1
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	enqueueQuota(t, s, 1, 1, 0)
	if _, _, err = s.Enqueue(ctx, quotaSnapshot(2, 2), 0, false); err == nil {
		t.Fatal("live quota ignored")
	}
	cfg = svc.Snapshot()
	cfg.AuditQuotas.OutstandingGlobal = 2
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	enqueueQuota(t, s, 2, 2, 0)
	cfg = svc.Snapshot()
	cfg.AuditQuotas.OutstandingGlobal = 1
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE status='pending'`).Scan(&count)
	if count != 2 {
		t.Fatal("lower quota deleted jobs")
	}
	reopened, err := OpenSettings(svc.path, "../../config.example.yaml")
	if err != nil || reopened.Snapshot().AuditQuotas.OutstandingGlobal != 1 {
		t.Fatal("quota not persisted", err)
	}
	omitted, err := svc.DecodePublic([]byte(`{}`))
	if err != nil || omitted.AuditQuotas != svc.Snapshot().AuditQuotas {
		t.Fatal("omitting quotas reset settings", err)
	}
	cfg = svc.Snapshot()
	cfg.AuditQuotas.OutstandingProject = 1
	if err = svc.Save(cfg); !errors.Is(err, ErrAuditQuotas) {
		t.Fatal("accepted running > outstanding", err)
	}
	cfg = svc.Snapshot()
	cfg.AuditQuotas.DailyGlobal = -1
	if err = svc.Save(cfg); !errors.Is(err, ErrAuditQuotas) {
		t.Fatal("negative quota accepted", err)
	}
}

func TestQuotaExpiryAfterLoweringLimitUsesRequiredExpirationNotOldest(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for n := 1; n <= 3; n++ {
		id := enqueueQuota(t, s, n, n, 0)
		if got, err := s.Claim(ctx); err != nil || got != id {
			t.Fatal(got, err)
		}
		if err := s.Finish(ctx, id, "succeeded", "", AuditResult{}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(`UPDATE platform_runs SET started_at=? WHERE id=?`, time.Now().UTC().Add(-time.Duration(n)*time.Hour).Format(time.RFC3339Nano), id); err != nil {
			t.Fatal(err)
		}
	}
	bindTestQuotas(s, AuditQuotas{DailyGlobal: 2})
	waiting := enqueueQuota(t, s, 4, 4, 0)
	run, _ := s.Run(ctx, waiting)
	wait, err := s.QueueWaitFor(ctx, run)
	if err != nil || wait == nil || wait.Reason != "global_daily" {
		t.Fatal(wait, err)
	}
	eligible, err := time.Parse(time.RFC3339Nano, wait.EligibleAt)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(eligible)
	if remaining < 22*time.Hour-5*time.Second || remaining > 22*time.Hour+5*time.Second {
		t.Fatal("reported oldest expiration while queue still blocked", remaining)
	}
}
func TestQuotaSystemStartsShareGlobalButNotPersonalBuckets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	bindTestQuotas(s, AuditQuotas{RunningUser: 1, DailyUser: 1, DailyGlobal: 2})
	one := enqueueQuota(t, s, 1, 1, 0)
	two := enqueueQuota(t, s, 2, 2, 0)
	three := enqueueQuota(t, s, 3, 3, 0)
	for _, want := range []int64{one, two} {
		if got, err := s.Claim(ctx); err != nil || got != want {
			t.Fatal("system incorrectly used personal quota", got, err)
		}
	}
	if _, err := s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("system escaped shared daily quota", err)
	}
	run, _ := s.Run(ctx, three)
	wait, err := s.QueueWaitFor(ctx, run)
	if err != nil || wait == nil || wait.Reason != "global_daily" {
		t.Fatal(wait, err)
	}
}
