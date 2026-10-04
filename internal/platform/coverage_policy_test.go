package platform

import (
	"context"
	"testing"
)

func TestIntentionalExclusionsDoNotImplyMissingCoverage(t *testing.T) {
	scope := BuildDiff([]Change{{OldPath: "README.md", NewPath: "README.md", Diff: "@@ -1 +1 @@\n-old\n+new"}, {OldPath: "code.any", NewPath: "code.any", Diff: "@@ -1 +1 @@\n-safe\n+danger"}}, []string{"md"}, 10000)
	if len(scope.Excluded) != 1 || len(scope.Notes) != 0 || scope.Text == "" || !scope.Added["code.any"][1] {
		t.Fatalf("scope classification: %+v", scope)
	}
	scope = BuildDiff([]Change{{NewPath: "data.bin", Diff: ""}}, nil, 10000)
	if len(scope.Notes) != 1 || len(scope.Excluded) != 0 {
		t.Fatal("unreadable data classified as intentional")
	}
}
func TestSkippedRunIsNotCleanAuditAndIsReusable(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	if err = s.Finish(ctx, id, "skipped", "", AuditResult{Summary: "excluded", ExcludedFiles: []string{"README.md"}}, nil); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "skipped" || len(run.Result.ExcludedFiles) != 1 {
		t.Fatal("skipped data not persisted")
	}
	next, created, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil || created || next != id {
		t.Fatal("repeat deliberate skip")
	}
}
