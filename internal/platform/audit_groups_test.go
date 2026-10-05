package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuditGroupPlannerDeterministicAndBounded(t *testing.T) {
	changes := []Change{}
	for i := 0; i < 60; i++ {
		changes = append(changes, Change{NewPath: fmt.Sprintf("pkg/f%03d.any", i), Diff: "@@ -1 +1 @@\n-old\n+new"})
	}
	first := PlanAuditGroups(changes, nil)
	for i, j := 0, len(changes)-1; i < j; i, j = i+1, j-1 {
		changes[i], changes[j] = changes[j], changes[i]
	}
	second := PlanAuditGroups(changes, nil)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) || len(first.Groups) != 3 {
		t.Fatal("unstable grouping", string(a), string(b))
	}
	seen := map[string]bool{}
	for _, g := range first.Groups {
		if len(g.Files) > auditGroupFiles || len(g.Scope.Text) > auditGroupBytes {
			t.Fatal("group limit bypass")
		}
		for _, p := range g.Files {
			if seen[p] || !g.Scope.Added[p][1] {
				t.Fatal("lost or duplicate anchor", p)
			}
			seen[p] = true
		}
	}
	if len(seen) != 60 || len(first.Notes) != 0 {
		t.Fatal("silent omission", first)
	}
}

func TestAuditGroupPlannerReportsExcludedAndOversized(t *testing.T) {
	changes := []Change{{NewPath: "readme.md", Diff: "@@ -1 +1 @@\n+doc"}, {NewPath: "large.any", Diff: strings.Repeat("x", auditGroupBytes+1)}}
	for i := 0; i < 10; i++ {
		changes = append(changes, Change{NewPath: fmt.Sprintf("f%d.any", i), Diff: "@@ -1 +1 @@\n+" + strings.Repeat("a", 20*1024)})
	}
	plan := PlanAuditGroups(changes, []string{"md"})
	if len(plan.Excluded) != 1 || plan.Excluded[0] != "readme.md" || len(plan.Notes) < 3 {
		t.Fatal("missing omission coverage", plan.Excluded, plan.Notes)
	}
	total := 0
	for _, g := range plan.Groups {
		total += len(g.Scope.Text)
	}
	if total > auditTotalDiffBytes {
		t.Fatal("total budget bypass", total)
	}
}

func TestAuditGroupPlannerGroupCapAndMetadata(t *testing.T) {
	changes := []Change{}
	for i := 0; i < 200; i++ {
		changes = append(changes, Change{NewPath: fmt.Sprintf("f%03d.any", i), Diff: "@@ -1 +1 @@\n+new"})
	}
	plan := PlanAuditGroups(changes, nil)
	if len(plan.Groups) != auditMaxGroups || len(plan.Notes) != 8 {
		t.Fatal("group count omission not tracked", len(plan.Groups), plan.Notes)
	}
	_, _, metadataChanges := metadataGitFixture(t)
	metadataPlan := PlanAuditGroups(metadataChanges, nil)
	found := false
	for _, g := range metadataPlan.Groups {
		if _, ok := g.Scope.Metadata["task.any"]; ok {
			found = true
		}
	}
	if !found {
		t.Fatal("metadata-only entry lost during grouping")
	}
}

func TestAuditGroupPlannerRecordsInputGapsOnce(t *testing.T) {
	plan := PlanAuditGroups([]Change{{NewPath: "file.any", Diff: "@@ -1 +1 @@\n+new", Notes: []string{"source metadata unavailable"}}}, nil)
	if len(plan.Notes) != 1 || len(plan.Groups) != 1 || len(plan.Groups[0].Scope.Notes) != 0 {
		t.Fatal("input gap duplicated in group scopes", plan)
	}
}
