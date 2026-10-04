package platform

import (
	"context"
	"errors"
	"testing"
)

func TestCheckpointSurvivesRestartAndDoesNotOverwriteCancellation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	result := AuditResult{Findings: []Finding{{ID: "validated"}}, Summary: "partial"}
	trace := []ToolTrace{{Name: "read_file", Output: "verified source"}}
	if err = s.Checkpoint(ctx, id, result, trace); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, id)
	if err != nil || r.Status != "failed" || len(r.Result.Findings) != 1 || len(r.Trace) != 1 {
		t.Fatal("restart erased checkpoint")
	}
	if err = s.Checkpoint(ctx, id, AuditResult{}, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("terminal result overwritten")
	}
	id, _, err = s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 2, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	s.Cancel(ctx, id, 1)
	if err = s.Checkpoint(ctx, id, result, trace); !errors.Is(err, ErrConflict) {
		t.Fatal("cancellation overwritten")
	}
}
