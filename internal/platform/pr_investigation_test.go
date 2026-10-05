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

func TestPRRiskRelationshipsRequireLinkedSources(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PRInvestigationContext)
	}{
		{"foreign", func(p *PRInvestigationContext) { p.Relationships[0].ObservationIDs = []string{"foreign"} }},
		{"no_source", func(p *PRInvestigationContext) { p.Relationships[0].ObservationIDs = nil }},
		{"false_certainty", func(p *PRInvestigationContext) { p.Relationships[0].Certainty = "runtime_proven" }},
		{"empty_endpoint", func(p *PRInvestigationContext) { p.Relationships[0].From = "" }},
		{"oversize", func(p *PRInvestigationContext) { p.Relationships[0].Relation = strings.Repeat("长", 501) }},
		{"unlinked_before", func(p *PRInvestigationContext) { p.BeforeObservationIDs = []string{"foreign"} }},
		{"unlinked_impact", func(p *PRInvestigationContext) { p.Impact[0].ObservationIDs = []string{"foreign"} }},
		{"unlinked_counterexample", func(p *PRInvestigationContext) { p.Counterexamples[0].ObservationIDs = []string{"foreign"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := riskRelationshipFixture()
			if err := validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source"}}); err != nil {
				t.Fatal(err)
			}
			tc.edit(p)
			if validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source"}}) == nil {
				t.Fatal("invalid risk chain accepted")
			}
		})
	}
	p := riskRelationshipFixture()
	copy := clonePRContext(p)
	p.Relationships[0].ObservationIDs[0] = "mutation"
	p.Impact[0].Statement = "mutation"
	p.BeforeObservationIDs[0] = "mutation"
	if copy.Relationships[0].ObservationIDs[0] != "source" || copy.Impact[0].Statement == "mutation" || copy.BeforeObservationIDs[0] != "source" {
		t.Fatal("risk chain not deeply copied")
	}
	copy.Relationships[0].Certainty = "inferred"
	if err := validatePRContext(Investigation{PRContext: copy, ObservationIDs: []string{"source"}}); err != nil {
		t.Fatal(err)
	}
}
func riskRelationshipFixture() *PRInvestigationContext {
	p := validPRContext("source")
	p.BeforeObservationIDs = []string{"source"}
	p.AfterObservationIDs = []string{"source"}
	p.Impact = []InvestigationFact{{Statement: "Unauthorized state change candidate", ObservationIDs: []string{"source"}}}
	p.Counterexamples = []InvestigationFact{{Statement: "Caller requires an authenticated user, ownership remains unknown", ObservationIDs: []string{"source"}}}
	p.Relationships = []InvestigationRelationship{{From: "API dispatch", To: "State update", Relation: "Dispatch argument maps order ID", Certainty: "cited", ObservationIDs: []string{"source"}}}
	return p
}

func TestPRSnapshotLinksRejectWrongSideAndContextRepository(t *testing.T) {
	tools := &auditTools{trace: []ToolTrace{
		{Name: "read_file", ObservationID: "head", Arguments: `{"base":false}`, Output: `{"text":"head"}`},
		{Name: "read_file", ObservationID: "base", Arguments: `{"base":true}`, Output: `{"text":"base"}`},
		{Name: "read_repository_file", ObservationID: "context", Output: `{"repository_id":2,"text":"context"}`},
		{Name: "get_diff", ObservationID: "diff", Output: `{"text":"diff"}`},
	}}
	if err := tools.validatePRSnapshotLinks(&PRInvestigationContext{BeforeObservationIDs: []string{"base"}, AfterObservationIDs: []string{"head"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"head", "context", "unknown"} {
		if tools.validatePRSnapshotLinks(&PRInvestigationContext{BeforeObservationIDs: []string{id}}) == nil {
			t.Fatal("wrong BASE reference", id)
		}
	}
	if tools.validatePRSnapshotLinks(&PRInvestigationContext{AfterObservationIDs: []string{"base"}}) == nil {
		t.Fatal("wrong HEAD reference")
	}
	if err := tools.validatePRSnapshotLinks(&PRInvestigationContext{BeforeObservationIDs: []string{"diff"}, AfterObservationIDs: []string{"diff"}}); err != nil {
		t.Fatal(err)
	}
}

func TestPRReferenceErrorsIdentifyRepairAndKeepSourceGate(t *testing.T) {
	p := riskRelationshipFixture()
	err := validatePRContext(Investigation{PRContext: p})
	if err == nil || !strings.Contains(err.Error(), `"source"`) || !strings.Contains(err.Error(), "observation_ids/counter_observation_ids") {
		t.Fatal("reference repair unclear", err)
	}
	if err := validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source"}}); err != nil {
		t.Fatal("explicit repair rejected", err)
	}
	p.EntryPoints[0].ObservationIDs = []string{"source", "source"}
	if err := validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source"}}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatal("duplicates not distinguished", err)
	}
	tools := &auditTools{}
	knowledge, _ := tools.riskChecklist(context.Background(), riskChecklistArgs{Category: "access_control"})
	out, _ := tools.record(context.Background(), Investigation{ID: "test", Claim: "unsupported", ObservationIDs: []string{knowledge.ObservationID}})
	if !strings.Contains(out.Error, knowledge.ObservationID) || !strings.Contains(out.Error, "evidence_eligible=true") || len(tools.ledger) > 0 {
		t.Fatal("source repair bypassed gate", out.Error)
	}
}
