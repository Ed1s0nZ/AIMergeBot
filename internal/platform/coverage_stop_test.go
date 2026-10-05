package platform

import (
	"context"
	"testing"
	"time"
)

type coverageStopAuditor struct{ err error }

func (a coverageStopAuditor) Audit(context.Context, Snapshot, DiffScope) (AuditResult, []ToolTrace, error) {
	return AuditResult{Summary: "partial investigation", Findings: []Finding{{ID: "retained"}}, Investigations: []Investigation{{ID: "unresolved", Status: "investigating", Claim: "caller unknown"}}, CoverageNotes: []string{"coverage stopped"}}, nil, a.err
}

func TestRunnerPersistsCoverageStopWithoutAutomaticRetry(t *testing.T) {
	for _, cause := range []error{ErrContextCompression, ErrModelTokenBudget, ErrModelUsageUnknown} {
		t.Run(cause.Error(), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
				t.Fatal(err)
			}
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "coverage-fixture", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			r := &Runner{Store: s, Repository: runRepo{}, Auditor: coverageStopAuditor{cause}, Timeout: time.Minute, active: map[int64]context.CancelFunc{}}
			id, _, err := r.Submit(ctx, 1, 1, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			r.execute(ctx, id)
			run, err := s.Run(ctx, id)
			if err != nil || run.Status != "incomplete" || len(run.Result.Findings) != 1 || len(run.Result.Investigations) != 1 || run.RetryChildID != 0 {
				t.Fatal("coverage stop lost result or retried", run, err)
			}
		})
	}
}
