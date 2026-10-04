package platform

import (
	"context"
	"testing"
)

func TestListSummaryDoesNotDecodeTrace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET trace_json='invalid expensive trace' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Run(ctx, id); err == nil {
		t.Fatal("fixture must fail full trace decoding")
	}
	run, err := s.RunSummary(ctx, id)
	if err != nil || len(run.Trace) != 0 || run.ID != id {
		t.Fatalf("summary loaded trace: %v", err)
	}
}
