package platform

import (
	"context"
	"strings"
	"testing"
)

func TestRunComparisonDoesNotInferFixOrMatchAmbiguousAnchors(t *testing.T) {
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	f := Finding{ID: "f", File: "a.go", Side: "head", Line: 4, Type: "injection", Evidence: "exact input", Trigger: "caller", Severity: "high"}
	f.Fingerprint = findingFingerprint(snap, f)
	prior := Run{ID: 1, Snapshot: snap, Status: "succeeded", Result: AuditResult{Findings: []Finding{f}}}
	current := Run{ID: 2, Snapshot: snap, Status: "incomplete", Result: AuditResult{Findings: []Finding{}}}
	out := compareRunFindings(current, prior)
	if len(out.Items) != 1 || out.Items[0].State != "not_reobserved" || len(out.Limitations) < 2 {
		t.Fatal(out)
	}
	current.Result.Findings = []Finding{f}
	current.Result.Findings[0].Severity = "medium"
	out = compareRunFindings(current, prior)
	if out.Items[0].State != "reobserved" || len(out.Items[0].Changed) != 1 || out.Items[0].Changed[0] != "severity" {
		t.Fatal(out)
	}
	current.Result.Findings = append(current.Result.Findings, f)
	out = compareRunFindings(current, prior)
	if len(out.Items) != 3 || out.Items[0].State != "unmatched" || out.Items[2].State != "not_reobserved" {
		t.Fatal(out)
	}
	current.Result.Findings = []Finding{f}
	current.Result.Findings[0].Fingerprint = "forged"
	out = compareRunFindings(current, prior)
	if out.Items[0].State != "unmatched" {
		t.Fatal("forged fingerprint matched")
	}
}
func TestCompareRunsAuthorizesBeforeReadingBodies(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "reader", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for i := 1; i <= 2; i++ {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: i, SourceProjectID: i, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		if _, err = s.DB.Exec(`UPDATE platform_runs SET trace_json='invalid' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.CompareRuns(ctx, ids[1], ids[0], u.ID); err == nil {
		t.Fatal("unauthorized comparison allowed")
	}
	if _, err = s.CompareRuns(ctx, ids[1], ids[0], 1); err != ErrComparisonScope {
		t.Fatalf("scope error: %v", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET project_id=1,source_project_id=1,head_sha='cccccccccccccccccccccccccccccccccccccccc' WHERE id=?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	out, err := s.CompareRuns(ctx, ids[1], ids[0], 1)
	if err != nil || out.Current.ID != ids[1] || len(out.Items) != 0 {
		t.Fatal(out, err)
	}
}
