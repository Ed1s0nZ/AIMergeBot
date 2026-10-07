package platform

import (
	"context"
	"strings"
	"testing"
)

func TestCheckRulesFailureCoverageAndThreshold(t *testing.T) {
	r := Run{ID: 7, Snapshot: Snapshot{HeadSHA: strings.Repeat("a", 40)}, Status: "succeeded", Result: AuditResult{Findings: []Finding{{Severity: "medium"}}}}
	p := CheckRules{Mode: "blocking", MinimumSeverity: "medium", BlockOnFailure: true, BlockOnIncomplete: true}
	if state, count := checkStateForRules(r, p); state != "failed" || count != 1 {
		t.Fatal(state, count)
	}
	p.MinimumSeverity = "high"
	if state, _ := checkStateForRules(r, p); state != "success" {
		t.Fatal(state)
	}
	r.Status = "failed"
	if state, _ := checkStateForRules(r, p); state != "failed" {
		t.Fatal(state)
	}
	p.BlockOnFailure = false
	if state, _ := checkStateForRules(r, p); state != "skipped" {
		t.Fatal(state)
	}
	r.Status = "incomplete"
	if state, _ := checkStateForRules(r, p); state != "failed" {
		t.Fatal(state)
	}
	p.BlockOnIncomplete = false
	if state, _ := checkStateForRules(r, p); state != "skipped" {
		t.Fatal(state)
	}
	r.Status = "succeeded"
	r.Result.Findings = []Finding{{Severity: "unexpected"}}
	if state, _ := checkStateForRules(r, p); state != "failed" {
		t.Fatal("unknown treated as pass", state)
	}
	p.Mode = "advisory"
	if state, _ := checkStateForRules(r, p); state != "skipped" {
		t.Fatal(state)
	}
	if defaults := effectiveCheckRules(Run{}); defaults.Enabled || defaults.Mode != "advisory" {
		t.Fatal("checks enabled by default", defaults)
	}
}

func TestCheckPolicyPublisherIsServerAssigned(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	input := WorkflowPolicy{Checks: &CheckRules{Enabled: true, Publisher: 999}}
	saved, err := s.SaveWorkflowPolicy(ctx, 1, 1, 0, input)
	if err != nil || saved.Checks.Publisher != 1 || saved.Checks.Mode != "advisory" || saved.Checks.MinimumSeverity != "high" {
		t.Fatal(saved, err)
	}
	if input.Checks.Publisher != 999 || input.Checks.Mode != "" {
		t.Fatal("caller policy mutated")
	}
	loaded, err := readWorkflowPolicy(ctx, s.DB, 1)
	if err != nil || loaded.Checks.Publisher != 1 {
		t.Fatal(loaded, err)
	}
	saved.Checks.Enabled = false
	saved, err = s.SaveWorkflowPolicy(ctx, 1, 1, 1, saved)
	if err != nil || saved.Checks.Publisher != 0 {
		t.Fatal("disabled policy retained publisher", saved, err)
	}
	for _, bad := range []CheckRules{{Mode: "unsafe"}, {MinimumSeverity: "unknown"}} {
		if validateWorkflowPolicy(WorkflowPolicy{Checks: &bad}) == nil {
			t.Fatal("invalid rules accepted")
		}
	}
}
