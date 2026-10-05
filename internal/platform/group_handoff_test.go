package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestGroupHandoffBoundedNavigationKeepsProvenance(t *testing.T) {
	snap := Snapshot{BaseSHA: "b", HeadSHA: "h"}
	result := AuditResult{Investigations: []Investigation{{ID: "group-1-risk", Claim: "guard unknown", Status: "investigating", ObservationIDs: []string{"old-1"}, NextSteps: []string{"read callers"}, PRContext: validPRContext("old-1")}}}
	out, _ := json.Marshal(toolOutput{BaseSHA: "b", HeadSHA: "h", Text: "PRIVATE SOURCE OUTPUT"})
	trace := []ToolTrace{{Name: "read_file", ObservationID: "old-1", Arguments: `{"path":"guard.any","base":true}`, Output: string(out)}}
	raw, omitted := buildGroupHandoff(snap, result, trace)
	var state groupHandoff
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatal(err)
	}
	if omitted || state.EvidenceEligible || len(state.Sources) != 1 || !strings.Contains(raw, "read callers") || strings.Contains(raw, "PRIVATE SOURCE OUTPUT") {
		t.Fatal("lost or misclassified notes", raw)
	}
	state.Investigations[0].PRContext.Before = "mutated"
	if result.Investigations[0].PRContext.Before == "mutated" {
		t.Fatal("handoff mutated live state")
	}
	fresh := &auditTools{snap: snap}
	if fresh.validateObservationIDs([]string{"old-1"}) == nil {
		t.Fatal("old notes became fresh group evidence")
	}
	result.Investigations = append(result.Investigations, Investigation{ID: "huge", Claim: strings.Repeat("x", groupHandoffBytes)})
	raw, omitted = buildGroupHandoff(snap, result, trace)
	if !omitted || len(raw) > groupHandoffBytes || !json.Valid([]byte(raw)) {
		t.Fatal("unbounded or malformed handoff")
	}
	trace[0].Error = "failed"
	raw, omitted = buildGroupHandoff(snap, result, trace)
	if !omitted || strings.Contains(raw, `"tool":"read_file"`) {
		t.Fatal("failed source locator included", raw)
	}
}
func TestGroupHandoffRevocationSuppressesPriorFacts(t *testing.T) {
	snap := Snapshot{AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "fixed"}}}}
	auditor := EinoAuditor{ContextSources: map[int]ContextSource{2: {Repository: fixtureRepo{}, Snapshot: Snapshot{ProjectID: 2, SourceProjectID: 2, BaseSHA: "fixed", HeadSHA: "fixed"}, Authorize: func(context.Context) error { return errors.New("revoked") }}}}
	raw, note := auditor.authorizedGroupHandoff(context.Background(), snap, AuditResult{Investigations: []Investigation{{Claim: "private fact"}}}, nil)
	if raw != "" || note == "" {
		t.Fatal("revoked facts passed", raw, note)
	}
}

func TestGroupHandoffRevocationDuringProjectionSuppressesFacts(t *testing.T) {
	snap := Snapshot{AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "fixed"}}}}
	calls := 0
	source := ContextSource{Repository: fixtureRepo{}, Snapshot: Snapshot{ProjectID: 2, SourceProjectID: 2, BaseSHA: "fixed", HeadSHA: "fixed"}, Authorize: func(context.Context) error {
		calls++
		if calls > 1 {
			return errors.New("revoked")
		}
		return nil
	}}
	auditor := EinoAuditor{ContextSources: map[int]ContextSource{2: source}}
	raw, note := auditor.authorizedGroupHandoff(context.Background(), snap, AuditResult{Investigations: []Investigation{{Claim: "private fact"}}}, nil)
	if calls != 2 || raw != "" || note == "" {
		t.Fatal("post-projection revocation ignored", calls, raw, note)
	}
}
