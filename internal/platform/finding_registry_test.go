package platform

import (
	"context"
	"testing"
)

func TestCanonicalFindingsRetainSubmissionsAndRejectInvalidProposals(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	ctx := context.Background()
	tools := &auditTools{repo: repo, snap: snap, scope: DiffScope{Added: map[string]map[int]bool{f.File: {f.Line: true}}}}
	if _, err := tools.submit(ctx, f); err != nil {
		t.Fatal(err)
	}
	accepted := tools.acceptedFindings()
	if len(accepted) != 1 {
		t.Fatal("submission not retained")
	}
	result := AuditResult{Findings: []Finding{}, Summary: "model omitted finding", CoverageNotes: []string{}}
	tools.mergeProposals(ctx, &result)
	if len(result.Findings) != 1 {
		t.Fatal("final omission erased accepted finding")
	}
	bad := f
	bad.Evidence = "invented"
	result.Findings = []Finding{bad}
	tools.mergeProposals(ctx, &result)
	if len(result.Findings) != 1 || len(result.CoverageNotes) != 1 {
		t.Fatal("invalid proposal erased valid evidence or was not reported")
	}
	result.Findings = []Finding{f, f}
	tools.mergeProposals(ctx, &result)
	if len(result.Findings) != 1 {
		t.Fatal("duplicate proposals")
	}
}
