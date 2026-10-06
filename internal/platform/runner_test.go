package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type runRepo struct{}

func (runRepo) Snapshot(_ context.Context, p, i int) (Snapshot, error) {
	return Snapshot{ProjectID: p, SourceProjectID: p, MRIID: i, HeadSHA: fmt.Sprint(i), BaseSHA: "base"}, nil
}
func (runRepo) Changes(context.Context, Snapshot) ([]Change, []string, error) {
	return []Change{{NewPath: "a.go", Diff: "@@ -0,0 +1 @@\n+new"}}, nil, nil
}
func (runRepo) ReadFile(context.Context, Snapshot, string, bool) (string, error) { return "new", nil }
func (runRepo) ListFiles(context.Context, Snapshot, int) ([]string, bool, error) {
	return []string{"a.go"}, false, nil
}

type blockingAuditor struct {
	started chan struct{}
	active  atomic.Int32
	max     atomic.Int32
}

func (a *blockingAuditor) Audit(ctx context.Context, _ Snapshot, _ DiffScope) (AuditResult, []ToolTrace, error) {
	n := a.active.Add(1)
	defer a.active.Add(-1)
	for {
		old := a.max.Load()
		if n <= old || a.max.CompareAndSwap(old, n) {
			break
		}
	}
	a.started <- struct{}{}
	<-ctx.Done()
	return AuditResult{}, nil, ctx.Err()
}
func waitStatus(t *testing.T, s *Store, id int64, status string, observationTimeout ...time.Duration) {
	t.Helper()
	limit := 3 * time.Second
	if len(observationTimeout) > 0 {
		limit = observationTimeout[0]
	}
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		r, e := s.Run(context.Background(), id)
		if e == nil && r.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, e := s.Run(context.Background(), id)
	t.Fatalf("want %s got %s (%v)", status, r.Status, e)
}
func TestRunnerBoundedCancellationAndTimeout(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "one", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	a := &blockingAuditor{started: make(chan struct{}, 5)}
	runner := &Runner{Store: s, Repository: runRepo{}, Auditor: a, Workers: 1, Timeout: 700 * time.Millisecond}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	id, _, err := runner.Submit(ctx, 1, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := runner.Submit(ctx, 1, 2, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if err = runner.Cancel(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, id, "cancelled")
	select {
	case <-a.started:
	case <-time.After(3 * time.Second):
		t.Fatal("second worker never ran")
	}
	waitStatus(t, s, second, "incomplete")
	if a.max.Load() != 1 {
		t.Fatal("concurrency exceeded configured worker count")
	}
}
func TestConcurrentEnqueueSingleClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, HeadSHA: "head", BaseSHA: "base"}
	done := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { _, _, err := s.Enqueue(ctx, snap, 0, false); done <- err }()
	}
	for i := 0; i < 12; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate run inserted")
	}
	if _, err := s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("double claim")
	}
}
