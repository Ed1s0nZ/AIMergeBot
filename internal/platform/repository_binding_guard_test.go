package platform

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type bindingGuardRepository struct {
	runRepo
	snapshots      atomic.Int32
	changes        atomic.Int32
	beforeSnapshot func()
}

func (r *bindingGuardRepository) Snapshot(ctx context.Context, p, i int) (Snapshot, error) {
	r.snapshots.Add(1)
	if r.beforeSnapshot != nil {
		r.beforeSnapshot()
	}
	return r.runRepo.Snapshot(ctx, p, i)
}
func (r *bindingGuardRepository) Changes(ctx context.Context, s Snapshot) ([]Change, []string, error) {
	r.changes.Add(1)
	return r.runRepo.Changes(ctx, s)
}
func bindGuardProject(t *testing.T, s *Store, admin int64, project int) {
	t.Helper()
	v := bindingIntegration(t, s, admin, "github", "https://api.example", project)
	if _, err := s.SaveRepositoryBinding(context.Background(), project, admin, 0, bindingValue(v, "github", "https://api.example")); err != nil {
		t.Fatal(err)
	}
}
func TestRepositoryBindingGuardSubmitBeforeAndAfterSnapshot(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "during"}[during], func(t *testing.T) {
			s, admin, _ := accessFixture(t)
			ctx := context.Background()
			repo := &bindingGuardRepository{}
			runner := &Runner{Store: s, Repository: repo}
			if during {
				repo.beforeSnapshot = func() { bindGuardProject(t, s, admin.ID, 1) }
			} else {
				bindGuardProject(t, s, admin.ID, 1)
			}
			if _, _, err := runner.Submit(ctx, 1, 1, admin.ID, false); !errors.Is(err, ErrRepositoryUnavailable) {
				t.Fatal(err)
			}
			want := int32(0)
			if during {
				want = 1
			}
			if repo.snapshots.Load() != want {
				t.Fatal(repo.snapshots.Load())
			}
			var count int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 0 {
				t.Fatal(count, err)
			}
		})
	}
	s, admin, _ := accessFixture(t)
	repo := &bindingGuardRepository{}
	runner := &Runner{Store: s, Repository: repo}
	if _, created, err := runner.Submit(context.Background(), 1, 1, admin.ID, false); err != nil || !created || repo.snapshots.Load() != 1 {
		t.Fatal(created, err)
	}
}

func TestRepositoryBindingGuardSnapshotScopesAndRecommendation(t *testing.T) {
	for _, project := range []int{1, 2, 3} {
		t.Run(string(rune('0'+project)), func(t *testing.T) {
			s, admin, viewer, run, snap, reader := ownerRecommendationFixture(t)
			ctx := context.Background()
			bindGuardProject(t, s, admin.ID, project)
			if _, _, err := s.EnqueueUser(ctx, snap, admin.ID, true); !errors.Is(err, ErrRepositoryUnavailable) {
				t.Fatal("enqueue bypass", err)
			}
			if _, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", reader); !errors.Is(err, ErrRepositoryUnavailable) || len(reader.reads) != 0 {
				t.Fatal("recommendation accessed legacy repository", reader.reads, err)
			}
			repo := &bindingGuardRepository{}
			runner := &Runner{Store: s, Repository: repo}
			if _, err := runner.Scope(ctx, run, viewer.ID); !errors.Is(err, ErrRepositoryUnavailable) || repo.changes.Load() != 0 {
				t.Fatal("scope accessed legacy repository", err)
			}
			if err := s.recordLegacyPollSeen(ctx, snap); !errors.Is(err, ErrRepositoryUnavailable) {
				t.Fatal("baseline bypass", err)
			}
			var count int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_poll_seen`).Scan(&count); err != nil || count != 0 {
				t.Fatal(count, err)
			}
			if _, err := s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			child, err := s.FailAndRetry(ctx, run, "temporary", AuditResult{Summary: "retained"}, nil, time.Second)
			parent, e := s.Run(ctx, run)
			if err != nil || e != nil || child != 0 || parent.Status != "failed" || parent.Result.Summary != "retained" || parent.RetryInfo == nil || parent.RetryInfo.State != "repository_unavailable" {
				t.Fatal(child, parent, err, e)
			}
		})
	}
}

func TestRepositoryBindingGuardQueuedExecutionDoesNotRead(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	repo := &bindingGuardRepository{}
	snap, _ := repo.runRepo.Snapshot(ctx, 1, 1)
	id, _, err := s.EnqueueUser(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	bindGuardProject(t, s, admin.ID, 1)
	auditor := &blockingAuditor{started: make(chan struct{}, 1)}
	runner := &Runner{Store: s, Repository: repo, Auditor: auditor, Workers: 1, Timeout: time.Minute}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	waitStatus(t, s, id, "failed")
	if repo.snapshots.Load() != 0 || repo.changes.Load() != 0 || len(auditor.started) != 0 {
		t.Fatal("bound execution reached repository or model")
	}
}

func TestRepositoryBindingGuardPollingHasNoRequestsOrSeen(t *testing.T) {
	s, _, runner, token := quotaIngressFixture(t)
	ctx := context.Background()
	admin, err := s.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	bindGuardProject(t, s, admin.ID, 1)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte(`[]`)) }))
	defer server.Close()
	cfg := runner.Settings.Snapshot()
	cfg.GitLab.URL = server.URL
	if err = runner.Settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	initialized := map[int]bool{}
	runner.pollCycle(ctx, initialized)
	if calls.Load() != 0 || initialized[1] {
		t.Fatal("bound project polled", calls.Load(), initialized)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_poll_seen`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestRepositoryBindingGuardPollingRejectsBindingDuringRead(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		t.Run(map[bool]string{false: "admit", true: "baseline"}[baseline], func(t *testing.T) {
			s, _, runner, token := quotaIngressFixture(t)
			ctx := context.Background()
			admin, err := s.Session(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			original := quotaGitLabFixture(t)
			var bound atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v4/projects/1/merge_requests/1/versions" && bound.CompareAndSwap(false, true) {
					bindGuardProject(t, s, admin.ID, 1)
				}
				original.Config.Handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			cfg := runner.Settings.Snapshot()
			cfg.GitLab.URL = server.URL
			cfg.ScanExistingMRs = !baseline
			if err = runner.Settings.Save(cfg); err != nil {
				t.Fatal(err)
			}
			initialized := map[int]bool{}
			runner.pollCycle(ctx, initialized)
			var count int
			for _, query := range []string{`SELECT COUNT(*) FROM platform_poll_seen`, `SELECT COUNT(*) FROM platform_runs`} {
				if err = s.DB.QueryRow(query).Scan(&count); err != nil || count != 0 {
					t.Fatal(query, count, err)
				}
			}
			if !bound.Load() || initialized[1] {
				t.Fatal("binding race not rejected", bound.Load(), initialized)
			}
		})
	}
}

func TestRepositoryBindingGuardHTTPSubmitIsUnavailableWithoutReads(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	_, token, err := s.Login(ctx, "admin", "admin-long-password")
	if err != nil {
		t.Fatal(err)
	}
	bindGuardProject(t, s, admin.ID, 1)
	repo := &bindingGuardRepository{}
	runner := &Runner{Store: s, Repository: repo}
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Runner: runner}).Register(router)
	for _, authenticated := range []bool{false, true} {
		req := httptest.NewRequest("POST", "/api/v1/runs", strings.NewReader(`{"project_id":1,"mr_iid":1}`))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		want := 401
		if authenticated {
			want = 503
		}
		if recorder.Code != want {
			t.Fatal(recorder.Code, recorder.Body.String())
		}
		if authenticated && (!strings.Contains(recorder.Body.String(), "repository_unavailable") || strings.Contains(recorder.Body.String(), "fixture-private-token") || recorder.Header().Get("Retry-After") != "") {
			t.Fatal(recorder.Body.String(), recorder.Header())
		}
	}
	if repo.snapshots.Load() != 0 {
		t.Fatal("HTTP used legacy reader")
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestRepositoryBindingGuardWebhookIsUnavailable(t *testing.T) {
	s, _, runner, token := quotaIngressFixture(t)
	ctx := context.Background()
	admin, err := s.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	bindGuardProject(t, s, admin.ID, 1)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	cfg := runner.Settings.Snapshot()
	cfg.GitLab.URL = server.URL
	if err = runner.Settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Runner: runner, Settings: runner.Settings}).Register(router)
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(`{"object_kind":"merge_request","project":{"id":1},"object_attributes":{"iid":1,"action":"update"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Token", "synthetic-webhook-token")
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != 503 || !strings.Contains(recorder.Body.String(), "repository_unavailable") || calls.Load() != 0 {
		t.Fatal(recorder.Code, recorder.Body.String(), calls.Load())
	}
}
