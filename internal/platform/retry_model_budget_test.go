package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryBudgetKnownRemainingAndUnknownStops(t *testing.T) {
	for _, tc := range []struct {
		name  string
		trace []ToolTrace
		want  string
		child bool
	}{
		{"known_remaining", []ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 6, CompletionTokens: 4, TotalTokens: 10}}, "scheduled", true},
		{"threshold", []ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 16, CompletionTokens: 4, TotalTokens: 20}}, "model_budget_exhausted", false},
		{"unknown", []ToolTrace{{Name: "model", Error: pendingModelUsage}}, "model_usage_unknown", false},
		{"no_model", []ToolTrace{{Name: "read_file"}}, "scheduled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{Model: "frozen", ModelBudget: ModelBudgetSettings{MaxTokens: 20}}}
			id, _, err := s.Enqueue(ctx, snap, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			s.Claim(ctx)
			child, err := s.FailAndRetry(ctx, id, "transient", AuditResult{Findings: []Finding{{ID: "retained"}}, Summary: "partial"}, tc.trace, 0)
			if err != nil || (child > 0) != tc.child {
				t.Fatal("retry creation", child, err)
			}
			parent, err := s.Run(ctx, id)
			if err != nil || parent.RetryInfo.State != tc.want || len(parent.Result.Findings) != 1 {
				t.Fatal("state or findings", err)
			}
			if tc.name == "known_remaining" {
				run, err := s.Run(ctx, child)
				if err != nil {
					t.Fatal(err)
				}
				seeded, err := s.seedRetryModelBudget(ctx, run)
				if err != nil {
					t.Fatal(err)
				}
				budget := seeded.Value(modelBudgetKey{}).(*modelTokenBudget)
				if budget.used != 10 || budget.limit != 20 {
					t.Fatal("child got fresh budget")
				}
				s.Claim(ctx)
				next, err := s.FailAndRetry(ctx, child, "transient", AuditResult{}, []ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 6, CompletionTokens: 4}}, 0)
				if err != nil || next != 0 {
					t.Fatal("cumulative threshold ignored", err)
				}
				chain, err := s.RetryChainUsage(ctx, id)
				if err != nil || len(chain.Attempts) != 2 || chain.Usage.PromptTokens != 12 || chain.Usage.CompletionTokens != 8 {
					t.Fatal("chain usage", chain, err)
				}
			}
		})
	}
}
func TestRecoveryBudgetUnknownRequestPreservesCheckpointWithoutChild(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{ModelBudget: ModelBudgetSettings{MaxTokens: 100}}}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	if err = s.Checkpoint(ctx, id, AuditResult{Summary: "checkpoint", Findings: []Finding{{ID: "retained"}}}, []ToolTrace{{Name: "model", Error: pendingModelUsage}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET worker_lease_until=? WHERE id=?`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
	child, err := s.recoverAttempt(ctx, id)
	if err != nil || child != 0 {
		t.Fatal("recovered unknown billing repeated", err)
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.RetryInfo.State != "model_usage_unknown" || len(run.Result.Findings) != 1 || run.Trace[0].Error != pendingModelUsage {
		t.Fatal("lost checkpoint", err)
	}
}
func TestRetryBudgetRejectsBrokenChain(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
	id, _, _ := s.Enqueue(ctx, snap, 1, false)
	s.Claim(ctx)
	child, err := s.FailAndRetry(ctx, id, "transient", AuditResult{}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE platform_runs SET project_id=2 WHERE id=?`, child)
	if _, err = s.RetryChainUsage(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatal("cross-project chain accepted", err)
	}
}

func TestRetryBudgetRejectsPolicyMutationWithUnchangedDigest(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{ModelBudget: ModelBudgetSettings{MaxTokens: 100}}}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	child, err := s.FailAndRetry(ctx, id, "transient", AuditResult{}, nil, 0)
	if err != nil || child == 0 {
		t.Fatal("create retry", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='{"model_budget":{"max_tokens":999}}' WHERE id=?`, child); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RetryChainUsage(ctx, id); !errors.Is(err, ErrConflict) {
		t.Fatal("mutated captured policy accepted", err)
	}
	run, err := s.Run(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.seedRetryModelBudget(ctx, run); !errors.Is(err, ErrConflict) {
		t.Fatal("mutated policy seeded model budget", err)
	}
}
