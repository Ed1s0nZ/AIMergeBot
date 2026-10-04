package platform

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func leaseStores(t *testing.T) (*Store, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shared.db")
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	return first, second
}
func expireInstance(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.DB.Exec(`UPDATE platform_worker_instance SET lease_until=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}
func TestWorkerInstanceConcurrentAcquisitionAcrossConnections(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, s := range []*Store{first, second} {
		go func(s *Store, owner string) { <-start; results <- s.AcquireWorkerInstance(ctx, owner) }(s, []string{"one", "two"}[i])
	}
	close(start)
	successful, rejected := 0, 0
	for n := 0; n < 2; n++ {
		err := <-results
		if err == nil {
			successful++
		} else if errors.Is(err, ErrWorkerInstanceActive) {
			rejected++
		} else {
			t.Fatal("transaction failed instead of serialized admission", err)
		}
	}
	if successful != 1 || rejected != 1 {
		t.Fatal(successful, rejected)
	}
}
func TestWorkerLiveOwnershipIsProtectedAndHiddenFromAPI(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	if err := first.AcquireWorkerInstance(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	id := enqueueQuota(t, first, 1, 1, 0)
	if got, err := first.ClaimOwned(ctx, "one"); err != nil || got != id {
		t.Fatal(got, err)
	}
	checkpoint := AuditResult{Summary: "checkpoint", Findings: []Finding{{ID: "retained"}}}
	trace := []ToolTrace{{ObservationID: "obs1", Name: "read_file"}}
	if err := first.CheckpointOwned(ctx, id, "one", checkpoint, trace); err != nil {
		t.Fatal(err)
	}
	if err := second.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Claim(ctx); !errors.Is(err, ErrWorkerInstanceActive) {
		t.Fatal("unowned claim bypassed live instance", err)
	}
	if err := second.AcquireWorkerInstance(ctx, "two"); !errors.Is(err, ErrWorkerInstanceActive) {
		t.Fatal(err)
	}
	for _, err := range []error{
		second.Checkpoint(ctx, id, AuditResult{}, nil),
		second.CheckpointOwned(ctx, id, "two", AuditResult{}, nil),
		second.FinishOwned(ctx, id, "two", "succeeded", "", AuditResult{}, nil),
		second.FailWorker(ctx, id, "two", "panic"),
	} {
		if !errors.Is(err, ErrConflict) {
			t.Fatal("foreign/unowned write accepted", err)
		}
	}
	if _, err := second.FailAndRetry(ctx, id, "temporary", AuditResult{}, nil, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("unowned retry accepted", err)
	}
	run, err := first.Run(ctx, id)
	if err != nil || run.Status != "running" || run.Result.Summary != "checkpoint" || len(run.Trace) != 1 || run.WorkerOwner != "one" {
		t.Fatal(run, err)
	}
	raw, err := json.Marshal(run)
	if err != nil || strings.Contains(string(raw), `"worker_owner"`) {
		t.Fatal("owner token exposed", err)
	}
	if err = first.RenewWorkerInstance(ctx, "one", id); err != nil {
		t.Fatal(err)
	}
	if err = first.OwnsRunningRun(ctx, id, "one"); err != nil {
		t.Fatal(err)
	}
}
func TestWorkerExpiredOwnerCannotRenewFinishOrDuplicateRecovery(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	if err := first.AcquireWorkerInstance(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	id := enqueueQuota(t, first, 1, 1, 0)
	first.ClaimOwned(ctx, "old")
	if err := first.CheckpointOwned(ctx, id, "old", AuditResult{Summary: "retained source proof"}, []ToolTrace{{Name: "read_file"}}); err != nil {
		t.Fatal(err)
	}
	expireInstance(t, first)
	if err := first.RenewWorkerInstance(ctx, "old"); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("expired owner revived", err)
	}
	if err := first.FinishOwned(ctx, id, "old", "succeeded", "", AuditResult{}, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("expired result persisted", err)
	}
	if err := second.AcquireWorkerInstance(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	// The copied run deadline is still in the future; the replaced instance is
	// nevertheless fenced. Two independent recovery transactions create one child.
	var wg sync.WaitGroup
	for _, s := range []*Store{first, second} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			if err := s.Recover(ctx); err != nil {
				t.Error(err)
			}
		}(s)
	}
	wg.Wait()
	run, err := second.Run(ctx, id)
	if err != nil || run.Status != "failed" || run.Result.Summary != "retained source proof" || len(run.Trace) != 1 {
		t.Fatal(run, err)
	}
	var count int
	if err = second.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE retry_parent_id=?`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate recovery child", count, err)
	}
	if err = first.ReleaseWorkerInstance(ctx, "old"); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("old stop released replacement", err)
	}
	var owner string
	var live bool
	second.DB.QueryRow(`SELECT owner,julianday(lease_until)>julianday('now') FROM platform_worker_instance`).Scan(&owner, &live)
	if owner != "new" || !live {
		t.Fatal("replacement corrupted")
	}
	for _, err := range []error{first.CheckpointOwned(ctx, id, "old", AuditResult{}, nil), first.FinishOwned(ctx, id, "old", "failed", "late", AuditResult{}, nil), first.FailWorker(ctx, id, "old", "late panic")} {
		if !errors.Is(err, ErrConflict) {
			t.Fatal("late write accepted", err)
		}
	}
	if _, err = first.FailAndRetryOwned(ctx, id, "old", "late temporary", AuditResult{}, nil, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("late retry accepted", err)
	}
	var events int
	second.DB.QueryRow(`SELECT COUNT(*) FROM platform_events WHERE action='run.recovered' AND target=?`, id).Scan(&events)
	if events != 1 {
		t.Fatal("recovery event not atomic", events)
	}
}
func TestWorkerExpiredRunLeaseIsNotRevivedByHeartbeat(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.AcquireWorkerInstance(ctx, "owner")
	id := enqueueQuota(t, s, 1, 1, 0)
	s.ClaimOwned(ctx, "owner")
	if _, err := s.DB.Exec(`UPDATE platform_runs SET worker_lease_until=? WHERE id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
	if err := s.RenewWorkerInstance(ctx, "owner", id); err != nil {
		t.Fatal(err)
	}
	if err := s.OwnsRunningRun(ctx, id, "owner"); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("expired run revived", err)
	}
	live, err := s.LiveWorkerContexts(ctx, "owner", []int64{id})
	if err != nil || live[id] {
		t.Fatal(live, err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Run(ctx, id)
	if run.Status != "failed" || run.RetryChildID == 0 {
		t.Fatal("expired run not recovered")
	}
}

type immediateAuditor struct{}

func (immediateAuditor) Audit(context.Context, Snapshot, DiffScope) (AuditResult, []ToolTrace, error) {
	return AuditResult{Summary: "synthetic recovery completed"}, nil, nil
}
func TestWorkerRunnerDuplicateStartAndGracefulRecovery(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	a := &blockingAuditor{started: make(chan struct{}, 1)}
	one := &Runner{Store: first, Repository: runRepo{}, Auditor: a, Workers: 1, Timeout: time.Minute}
	two := &Runner{Store: second, Repository: runRepo{}, Auditor: immediateAuditor{}, Workers: 1, Timeout: time.Minute}
	if err := one.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer one.Stop()
	id := enqueueQuota(t, first, 1, 1, 0)
	select {
	case <-a.started:
	case <-time.After(3 * time.Second):
		t.Fatal("audit did not start")
	}
	if err := two.Start(ctx); !errors.Is(err, ErrWorkerInstanceActive) {
		t.Fatal("second startup recovered live work", err)
	}
	if err := one.Start(ctx); err == nil {
		t.Fatal("same runner started twice")
	}
	if err := first.CheckpointOwned(ctx, id, one.owner, AuditResult{Summary: "durable before stop"}, []ToolTrace{{Name: "read_file"}}); err != nil {
		t.Fatal(err)
	}
	if err := second.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ := second.Run(ctx, id)
	if run.Status != "running" || run.RetryChildID != 0 {
		t.Fatal("live run recovered")
	}
	one.Stop()
	one.Stop()
	run, _ = second.Run(ctx, id)
	if run.Status != "running" || run.Result.Summary != "durable before stop" {
		t.Fatal("graceful stop erased resumable progress", run.Status, run.Result.Summary)
	}
	if err := two.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer two.Stop()
	run, _ = second.Run(ctx, id)
	if run.Status != "failed" || run.RetryChildID == 0 || run.Result.Summary != "durable before stop" {
		t.Fatal("restart failed to preserve/recover checkpoint", run)
	}
	if _, err := second.DB.Exec(`UPDATE platform_runs SET retry_at='' WHERE id=?`, run.RetryChildID); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, second, run.RetryChildID, "succeeded")
	child, _ := second.Run(ctx, run.RetryChildID)
	if child.RetryAttempt != 1 || child.WorkerOwner == run.WorkerOwner {
		t.Fatal("retry retained old worker or reset retry bound")
	}
}
func TestWorkerOwnerLossCancelsWorkReportsFailureAndProtectsReplacement(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	a := &blockingAuditor{started: make(chan struct{}, 1)}
	r := &Runner{Store: first, Repository: runRepo{}, Auditor: a, Workers: 1, Timeout: time.Minute}
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	id := enqueueQuota(t, first, 1, 1, 0)
	select {
	case <-a.started:
	case <-time.After(3 * time.Second):
		t.Fatal("audit did not start")
	}
	expireInstance(t, first)
	if err := second.AcquireWorkerInstance(ctx, "replacement"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-r.Failures():
		if !errors.Is(err, ErrWorkerLeaseLost) {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("ownership loss was not reported to host")
	}
	deadline := time.Now().Add(3 * time.Second)
	for a.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.active.Load() != 0 {
		t.Fatal("old worker was not cancelled")
	}
	if _, _, err := r.Submit(ctx, 1, 1, 0, false); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("lost owner still accepted requests", err)
	}
	r.Stop()
	var live bool
	var owner string
	if err := second.DB.QueryRow(`SELECT owner,julianday(lease_until)>julianday('now') FROM platform_worker_instance`).Scan(&owner, &live); err != nil || owner != "replacement" || !live {
		t.Fatal("old stop corrupted new owner", owner, live, err)
	}
	run, _ := second.Run(ctx, id)
	if run.Status != "running" {
		t.Fatal("lost owner replaced result instead of leaving recovery", run.Status)
	}
}

func TestWorkerAbandonedResultWriteDoesNotLeaveImmortalRunningAttempt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.AcquireWorkerInstance(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	id := enqueueQuota(t, s, 1, 1, 0)
	if _, err := s.ClaimOwned(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckpointOwned(ctx, id, "owner", AuditResult{Summary: "checkpoint before storage failure"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER synthetic_finish_failure BEFORE UPDATE OF status ON platform_runs WHEN NEW.status='succeeded' BEGIN SELECT RAISE(ABORT,'synthetic terminal write failure');END`); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Store: s, Repository: runRepo{}, Auditor: immediateAuditor{}, Timeout: time.Minute, owner: "owner", active: map[int64]context.CancelFunc{}}
	r.execute(ctx, id)
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "running" || run.Result.Summary != "checkpoint before storage failure" {
		t.Fatal("failed persistence erased checkpoint", run, err)
	}
	if err = s.OwnsRunningRun(ctx, id, "owner"); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("abandoned job retained renewable lease", err)
	}
	if _, err = s.DB.Exec(`DROP TRIGGER synthetic_finish_failure`); err != nil {
		t.Fatal(err)
	}
	if err = r.maintainLease(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ = s.Run(ctx, id)
	if run.Status != "failed" || run.RetryChildID == 0 || run.Result.Summary != "checkpoint before storage failure" {
		t.Fatal("heartbeat failed to recover abandoned execution")
	}
}

type leasePanicAuditor struct{ action func() }

func (a leasePanicAuditor) Audit(context.Context, Snapshot, DiffScope) (AuditResult, []ToolTrace, error) {
	a.action()
	panic("synthetic stale worker panic")
}
func TestWorkerStalePanicCannotReplaceCheckpoint(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	if err := first.AcquireWorkerInstance(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	id := enqueueQuota(t, first, 1, 1, 0)
	first.ClaimOwned(ctx, "old")
	if err := first.CheckpointOwned(ctx, id, "old", AuditResult{Summary: "proof before stale panic"}, []ToolTrace{{Name: "read_file"}}); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Store: first, Repository: runRepo{}, Auditor: leasePanicAuditor{action: func() {
		expireInstance(t, first)
		if err := second.AcquireWorkerInstance(ctx, "new"); err != nil {
			t.Fatal(err)
		}
	}}, Timeout: time.Minute, owner: "old", active: map[int64]context.CancelFunc{}}
	r.execute(ctx, id)
	run, err := second.Run(ctx, id)
	if err != nil || run.Status != "running" || run.Result.Summary != "proof before stale panic" || len(run.Trace) != 1 {
		t.Fatal("stale panic wrote over live checkpoint", run, err)
	}
	if err = second.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ = second.Run(ctx, id)
	if run.Status != "failed" || run.RetryChildID == 0 || run.Result.Summary != "proof before stale panic" {
		t.Fatal("stale panic broke later recovery")
	}
}
func TestWorkerCancellationWinsExpiredRecovery(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	first.AcquireWorkerInstance(ctx, "old")
	id := enqueueQuota(t, first, 1, 1, 0)
	first.ClaimOwned(ctx, "old")
	expireInstance(t, first)
	if err := second.AcquireWorkerInstance(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	if err := second.Cancel(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	if err := second.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ := second.Run(ctx, id)
	if run.Status != "cancelled" || run.RetryChildID != 0 {
		t.Fatal("recovery revived cancelled audit")
	}
}

func TestWorkerCommentReservationAndAcknowledgementAreFenced(t *testing.T) {
	first, second := leaseStores(t)
	ctx := context.Background()
	if err := first.AcquireWorkerInstance(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	id := enqueueQuota(t, first, 1, 1, 0)
	if _, err := first.ClaimOwned(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	if err := first.FinishOwned(ctx, id, "old", "succeeded", "", AuditResult{}, nil); err != nil {
		t.Fatal(err)
	}
	if claimed, err := first.claimOwnedComment(ctx, id, "old"); err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	expireInstance(t, first)
	if err := second.AcquireWorkerInstance(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	if first.commentOwnerValid(ctx, id, "old") {
		t.Fatal("stale worker can initiate external comment")
	}
	if claimed, err := second.claimOwnedComment(ctx, id, "new"); err != nil || claimed {
		t.Fatal("replacement claimed old completed comment", claimed, err)
	}
	if saved, err := first.finishOwnedComment(ctx, id, "old", "sent"); err != nil || saved {
		t.Fatal("stale acknowledgement replaced uncertain delivery", saved, err)
	}
	var status string
	if err := second.DB.QueryRow(`SELECT status FROM platform_comments WHERE run_id=?`, id).Scan(&status); err != nil || status != "sending" {
		t.Fatal("uncertain delivery was silently rewritten", status, err)
	}
}
