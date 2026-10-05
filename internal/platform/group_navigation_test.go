package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGroupNavigationBoundedAndSingleAuditUnchanged(t *testing.T) {
	if currentGroupNavigation(nil) != "" {
		t.Fatal("single audit gained group instructions")
	}
	group := AuditGroup{ID: "group-2", Files: []string{"current.any"}}
	note := currentGroupNavigation(&group)
	if !strings.Contains(note, `"files":["current.any"]`) || !strings.Contains(note, "do not resubmit") {
		t.Fatal("current scope missing")
	}
	group.Files = []string{strings.Repeat("界", 10000)}
	note = currentGroupNavigation(&group)
	parts := strings.Split(note, "\n")
	if len(note) > 17*1024 || !json.Valid([]byte(parts[2])) || !strings.Contains(parts[2], "files_omitted") {
		t.Fatal("oversized navigation not bounded JSON")
	}
}

func TestCoverageDeduplicationKeepsDistinctGapsAndOrder(t *testing.T) {
	notes := []string{"group1 missing source", "group2 missing source", "group1 missing source", " group1 missing source"}
	got := uniqueCoverageNotes(notes)
	if len(got) != 3 || got[0] != notes[0] || got[1] != notes[1] || got[2] != notes[3] {
		t.Fatal("distinct gaps lost or order changed", got)
	}
	got[0] = "changed"
	if notes[0] != "group1 missing source" {
		t.Fatal("projection mutated original coverage")
	}
}

func TestFindingOutsideGroupScopeCannotBorrowWholePRAnchor(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	result := AuditResult{Findings: []Finding{f}}
	scope := DiffScope{Added: map[string]map[int]bool{"current.any": {1: true}}}
	err := ValidateFindings(context.Background(), repo, snap, scope, &result)
	if err == nil || !strings.Contains(err.Error(), "outside the current audit scope") || !strings.Contains(err.Error(), "without resubmitting") {
		t.Fatal("cross-group submission not rejected clearly", err)
	}
}
