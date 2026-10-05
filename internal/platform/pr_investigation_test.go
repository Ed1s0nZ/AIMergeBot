package platform

import (
	"context"
	"strings"
	"testing"
)

func validPRContext(id string) *PRInvestigationContext {
	return &PRInvestigationContext{ChangeSummary: "PR removes a resource guard", Before: "Ownership checked", After: "Operation no longer checks ownership", EntryPoints: []InvestigationFact{{Statement: "Changed operation", ObservationIDs: []string{id}}}, UnresolvedEdges: []string{"Caller authentication remains to inspect"}}
}
func TestPRContextValidation(t *testing.T) {
	if err := validatePRContext(Investigation{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*PRInvestigationContext)
	}{
		{"empty_before", func(p *PRInvestigationContext) { p.Before = "" }},
		{"oversized", func(p *PRInvestigationContext) { p.After = strings.Repeat("x", 801) }},
		{"unlinked", func(p *PRInvestigationContext) { p.EntryPoints[0].ObservationIDs = []string{"other"} }},
		{"duplicate", func(p *PRInvestigationContext) { p.EntryPoints[0].ObservationIDs = []string{"source", "source"} }},
		{"no_source", func(p *PRInvestigationContext) { p.EntryPoints[0].ObservationIDs = nil }},
		{"blank_edge", func(p *PRInvestigationContext) { p.UnresolvedEdges = []string{" "} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPRContext("source")
			tc.edit(p)
			if validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source"}}) == nil {
				t.Fatal("invalid context accepted")
			}
		})
	}
	if err := validatePRContext(Investigation{PRContext: validPRContext("counter"), CounterObservationIDs: []string{"counter"}}); err != nil {
		t.Fatal(err)
	}
}
func TestPRContextLedgerAndFindingSnapshot(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	source, _ := tools.file(context.Background(), readArgs{Path: f.File, Start: 1, End: 3})
	if source.Error != "" {
		t.Fatal(source.Error)
	}
	p := validPRContext(source.ObservationID)
	out, _ := tools.record(context.Background(), Investigation{ID: "impact", Claim: "changed behavior", ObservationIDs: []string{source.ObservationID}, PRContext: p})
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	p.Before = "mutated by caller"
	if tools.ledger["impact"].PRContext.Before == p.Before {
		t.Fatal("ledger retained caller pointer")
	}
	f.InvestigationID = "impact"
	f.PRContext = &PRInvestigationContext{Before: "unvalidated proposal"}
	accepted, err := tools.acceptFinding(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.PRContext == nil || accepted.PRContext.Before != "Ownership checked" {
		t.Fatal("finding did not derive validated context")
	}
	tools.ledger["impact"].PRContext.Before = "later ledger change"
	if accepted.PRContext.Before != "Ownership checked" {
		t.Fatal("accepted finding context changed")
	}
}
func TestPRContextRejectsKnowledgeObservationAndPreservesLedger(t *testing.T) {
	tools := &auditTools{}
	guidance, _ := tools.riskChecklist(context.Background(), riskChecklistArgs{Category: "access_control"})
	out, _ := tools.record(context.Background(), Investigation{ID: "impact", Claim: "test", ObservationIDs: []string{guidance.ObservationID}, PRContext: validPRContext(guidance.ObservationID)})
	if out.Error == "" || len(tools.ledger) != 0 {
		t.Fatal("guidance became source evidence")
	}
	out, _ = tools.record(context.Background(), Investigation{ID: "impact", Claim: "original"})
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	out, _ = tools.update(context.Background(), Investigation{ID: "impact", Claim: "replacement", Status: "investigating", PRContext: &PRInvestigationContext{}})
	if out.Error == "" || tools.ledger["impact"].Claim != "original" {
		t.Fatal("invalid update changed ledger")
	}
}
