package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	modelopenai "github.com/meguminnnnnnnnn/go-openai"
	"testing"
	"time"
)

func TestRetryClassificationUsesTypedErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 502, 503} {
		err := fmt.Errorf("wrapped: %w", &modelopenai.APIError{HTTPStatusCode: status})
		want := status == 429 || status >= 500
		if retryableError(err) != want {
			t.Fatalf("status %d", status)
		}
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, errors.New("invented output status code: 503")} {
		if retryableError(err) {
			t.Fatal("untyped/permanent error retried")
		}
	}
}
func TestRetriesAreDelayedBoundedAndPreserveAttempts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{Model: "frozen"}}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	result := AuditResult{Summary: "validated partial", Findings: []Finding{{ID: "retained"}}}
	child, err := s.FailAndRetry(ctx, id, "temporary", result, []ToolTrace{{Name: "read_file"}}, time.Hour)
	if err != nil || child == 0 {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("delayed retry claimed early")
	}
	parent, err := s.Run(ctx, id)
	if err != nil || parent.Status != "failed" || len(parent.Result.Findings) != 1 {
		t.Fatal("parent result lost")
	}
	retry, err := s.Run(ctx, child)
	if err != nil || retry.RetryAttempt != 1 || retry.RetryParentID != id || retry.AuditPolicy.Model != "frozen" {
		t.Fatal("child does not inherit pinned identity")
	}
	s.DB.Exec(`UPDATE platform_runs SET retry_at='' WHERE id=?`, child)
	s.Claim(ctx)
	grandchild, err := s.FailAndRetry(ctx, child, "temporary", AuditResult{}, nil, 0)
	if err != nil || grandchild == 0 {
		t.Fatal("second retry missing")
	}
	s.Claim(ctx)
	last, err := s.FailAndRetry(ctx, grandchild, "temporary", AuditResult{}, nil, 0)
	if err != nil || last != 0 {
		t.Fatal("retry bound exceeded")
	}
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("extra retry queued")
	}
}
func TestCancelledAttemptCannotScheduleRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	s.Cancel(ctx, id, 1)
	if _, err = s.FailAndRetry(ctx, id, "temporary", AuditResult{}, nil, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled attempt revived")
	}
}

type transientAuditor struct{ calls int }

func (a *transientAuditor) Audit(context.Context, Snapshot, DiffScope) (AuditResult, []ToolTrace, error) {
	a.calls++
	if a.calls == 1 {
		return AuditResult{}, nil, &modelopenai.APIError{HTTPStatusCode: 429}
	}
	return AuditResult{Summary: "recovered", Findings: []Finding{}, CoverageNotes: []string{}}, nil, nil
}
func TestRunnerSchedulesAndExecutesTransientRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "retry-fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	a := &transientAuditor{}
	r := &Runner{Store: s, Repository: runRepo{}, Auditor: a, Timeout: time.Minute, active: map[int64]context.CancelFunc{}}
	id, _, err := r.Submit(ctx, 1, 1, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	r.execute(ctx, id)
	var child int64
	if err = s.DB.QueryRow(`SELECT id FROM platform_runs WHERE retry_parent_id=?`, id).Scan(&child); err != nil {
		t.Fatal("worker did not schedule retry", err)
	}
	s.DB.Exec(`UPDATE platform_runs SET retry_at='' WHERE id=?`, child)
	claimed, err := s.Claim(ctx)
	if err != nil || claimed != child {
		t.Fatal("child cannot be claimed", err)
	}
	r.execute(ctx, child)
	run, err := s.Run(ctx, child)
	if err != nil || run.Status != "succeeded" || a.calls != 2 {
		t.Fatal("retry did not recover", err)
	}
}
