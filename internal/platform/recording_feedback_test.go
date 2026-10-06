package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRecordingFeedbackIsBoundedAndDoesNotMutate(t *testing.T) {
	a := Investigation{Plan: pendingPlan(), PRContext: &PRInvestigationContext{Before: "private claim", After: "private claim"}}
	before, _ := json.Marshal(a)
	want := []string{"plan_unfinished", "base_sources_missing", "head_sources_missing", "relationships_missing"}
	if got := investigationRecordingGaps(a); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("feedback mutated claim")
	}
	if got := prRecordingGaps(nil, true); len(got) != 0 {
		t.Fatal("canonical metadata forced runtime path", got)
	}
	a.PRContext.BeforeObservationIDs = []string{"base"}
	a.PRContext.AfterObservationIDs = []string{"head"}
	a.PRContext.Relationships = []InvestigationRelationship{{Certainty: "inferred"}}
	if got := prRecordingGaps(a.PRContext, false); !reflect.DeepEqual(got, []string{"relationships_inferred"}) {
		t.Fatal("inferred path hidden", got)
	}
}
func TestLedgerFeedbackIsNotEvidenceAndRetainsSourceGates(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	ctx := context.Background()
	source, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 3})
	a := Investigation{ID: "feedback", Claim: "changed behavior", ObservationIDs: []string{source.ObservationID}, PRContext: validPRContext(source.ObservationID)}
	out, _ := tools.record(ctx, a)
	if out.Error != "" || out.EvidenceEligible || len(out.RecordingGaps) != 4 {
		t.Fatal("missing early gaps", out)
	}
	tools.mu.Lock()
	err := tools.validateObservationIDs([]string{out.ObservationID})
	tools.mu.Unlock()
	if err == nil {
		t.Fatal("feedback accepted as source")
	}
	a.Status = "investigating"
	a.PRContext.BeforeObservationIDs = []string{source.ObservationID}
	bad, _ := tools.update(ctx, a)
	if bad.Error == "" || len(bad.RecordingGaps) != 0 {
		t.Fatal("wrong-side source accepted", bad)
	}
	f.InvestigationID = a.ID
	submitted, _ := tools.submit(ctx, f)
	if submitted.Error != "" || submitted.EvidenceEligible || len(submitted.RecordingGaps) != 3 {
		t.Fatal("submit did not expose accepted snapshot gaps", submitted)
	}
	base, _ := tools.file(ctx, readArgs{Path: f.File, Base: true, Start: 1, End: 3})
	a.ObservationIDs = []string{source.ObservationID, base.ObservationID}
	a.PRContext.BeforeObservationIDs = []string{base.ObservationID}
	a.PRContext.AfterObservationIDs = []string{source.ObservationID}
	a.PRContext.Relationships = []InvestigationRelationship{{From: "input", To: "operation", Relation: "fixture static relationship", Certainty: "cited", ObservationIDs: []string{source.ObservationID}}}
	a.Plan = pendingPlan()
	for i := range a.Plan {
		a.Plan[i].Status = "checked"
		a.Plan[i].Reason = "fixture inspection only"
		a.Plan[i].ObservationIDs = []string{source.ObservationID}
	}
	corrected, _ := tools.update(ctx, a)
	if corrected.Error != "" || len(corrected.RecordingGaps) != 0 {
		t.Fatal("valid recording correction still has gaps", corrected)
	}
	// Submitted findings retain their earlier context until deliberately resubmitted.
	if len(prRecordingGaps(tools.acceptedFindings()[0].PRContext, false)) != 3 {
		t.Fatal("feedback silently upgraded prior finding")
	}
	resubmitted, _ := tools.submit(ctx, f)
	if resubmitted.Error != "" || len(resubmitted.RecordingGaps) != 0 {
		t.Fatal("resubmission did not use validated ledger", resubmitted)
	}
}
