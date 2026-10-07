package platform

import (
	"strings"
	"testing"
)

func TestRunCheckAssessmentPreservesCommitAndFailureStates(t *testing.T) {
	for _, status := range []string{"pending", "running", "failed", "cancelled", "skipped", "incomplete"} {
		r := Run{ID: 7, Snapshot: Snapshot{HeadSHA: strings.Repeat("a", 40)}, Status: status, Result: AuditResult{Findings: []Finding{{Severity: "high"}}}}
		a := assessRunCheck(r)
		if a.State != status || a.RunID != 7 || a.HeadSHA != r.HeadSHA || a.HighRiskFindings != 1 || a.Published || a.Blocking {
			t.Fatal(a)
		}
	}
	r := Run{ID: 7, Snapshot: Snapshot{HeadSHA: strings.Repeat("a", 40)}, Status: "succeeded"}
	if assessRunCheck(r).State != "completed" {
		t.Fatal("complete run not recognized")
	}
	r.Result.Findings = []Finding{{Severity: "critical"}, {Severity: "high"}, {Severity: "medium"}}
	if a := assessRunCheck(r); a.State != "high_risk" || a.HighRiskFindings != 2 {
		t.Fatal(a)
	}
	r.Result.CoverageNotes = []string{"missing callers"}
	if assessRunCheck(r).State != "incomplete" {
		t.Fatal("incomplete hidden by risk")
	}
	r.Error = "transport failure"
	if assessRunCheck(r).State != "failed" {
		t.Fatal("error hidden by completion")
	}
	r.Error = ""
	r.Result.CoverageNotes = nil
	r.Result.Findings = []Finding{{Severity: "unexpected"}}
	if assessRunCheck(r).State != "unknown" {
		t.Fatal("unknown severity treated as clear")
	}
	r.Result.Findings = nil
	r.HeadSHA = "missing"
	if assessRunCheck(r).State != "unknown" {
		t.Fatal("invalid commit treated as complete check")
	}
	r.Status = "unexpected"
	if assessRunCheck(r).State != "unknown" {
		t.Fatal("unknown state treated as complete")
	}
}
