package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func contextRecordingFixture(t *testing.T) (*auditTools, prContextRecording, Finding) {
	t.Helper()
	repo, snap, f, sources := crossGitFixture(t)
	tools := crossTools(repo, snap, sources)
	ctx := context.Background()
	base, _ := tools.file(ctx, readArgs{Path: f.File, Base: true, Start: 1, End: 3})
	head, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 3})
	related, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: f.File, Start: 1, End: 3})
	for _, o := range []toolOutput{base, head, related} {
		if o.Error != "" {
			t.Fatal(o.Error)
		}
	}
	inv := Investigation{ID: "inspection", Claim: "Fixed source changes behavior", Status: "supported", Evidence: []string{"Original evidence"}, Counterevidence: []string{"Original counterevidence"}, NextSteps: []string{"Keep actual unknown"}, ObservationIDs: []string{head.ObservationID}}
	for i, k := range verificationKinds {
		inv.Plan = append(inv.Plan, InvestigationTask{ID: []string{"input", "cause", "guard", "outcome"}[i], Kind: k, Question: "Original question", Status: "checked", Reason: "Inspected source", ObservationIDs: []string{head.ObservationID}})
	}
	tools.ledger = map[string]Investigation{inv.ID: inv}
	p := validPRContext(head.ObservationID)
	p.BeforeObservationIDs = []string{base.ObservationID}
	p.AfterObservationIDs = []string{head.ObservationID}
	p.Relationships = []InvestigationRelationship{{From: "Primary operation", To: "Fixed context contract", Relation: "Inspected relation remains a candidate", Certainty: "inferred", ObservationIDs: []string{head.ObservationID, related.ObservationID}}}
	return tools, prContextRecording{ID: inv.ID, Claim: inv.Claim, PRContext: p, ObservationIDs: []string{base.ObservationID, head.ObservationID, related.ObservationID}}, f
}

func TestRecordPRContextPreservesJudgmentAndFindingSnapshot(t *testing.T) {
	tools, a, f := contextRecordingFixture(t)
	original := tools.ledger[a.ID]
	f.InvestigationID = a.ID
	f.ObservationIDs = slices.Clone(original.ObservationIDs)
	accepted, err := tools.acceptFinding(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := tools.recordPRContext(context.Background(), a)
	if out.Error != "" || out.EvidenceEligible {
		t.Fatal(out)
	}
	got := tools.ledger[a.ID]
	saved := got
	saved.PRContext = original.PRContext
	saved.ObservationIDs = original.ObservationIDs
	if !reflect.DeepEqual(saved, original) {
		t.Fatal("unrelated investigation fields changed", saved, original)
	}
	if !slices.Contains(out.RecordingGaps, "relationships_inferred") || slices.Contains(out.RecordingGaps, "base_sources_missing") {
		t.Fatal(out.RecordingGaps)
	}
	if accepted.PRContext != nil || tools.findings[f.ID].PRContext != nil {
		t.Fatal("accepted finding silently updated")
	}
	resubmitted, err := tools.acceptFinding(context.Background(), f)
	if err != nil || resubmitted.PRContext == nil {
		t.Fatal(err, resubmitted)
	}
	a.PRContext.Before = "mutated caller data"
	if tools.ledger[a.ID].PRContext.Before == a.PRContext.Before {
		t.Fatal("caller pointer retained")
	}
	if len(tools.investigationPRCoverage(tools.investigations())) == 0 {
		t.Fatal("inferred edge became completeness")
	}
	if tools.trace[len(tools.trace)-1].Name != "record_pr_context" {
		t.Fatal("missing actual tool trace")
	}
}

func TestRecordPRContextFailuresAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*auditTools, *prContextRecording)
	}{
		{"missing_context", func(_ *auditTools, a *prContextRecording) { a.PRContext = nil }},
		{"empty_relationship_target", func(_ *auditTools, a *prContextRecording) { a.PRContext.Relationships[0].To = "" }},
		{"invalid_relationship_certainty", func(_ *auditTools, a *prContextRecording) { a.PRContext.Relationships[0].Certainty = "unknown" }},
		{"unknown_id", func(_ *auditTools, a *prContextRecording) { a.ID = "unknown" }},
		{"claim_changed", func(_ *auditTools, a *prContextRecording) { a.Claim = "different claim" }},
		{"wrong_side", func(_ *auditTools, a *prContextRecording) {
			a.PRContext.BeforeObservationIDs = a.PRContext.AfterObservationIDs
		}},
		{"context_as_primary", func(_ *auditTools, a *prContextRecording) {
			a.PRContext.AfterObservationIDs = []string{a.ObservationIDs[2]}
		}},
		{"no_automatic_id_link", func(_ *auditTools, a *prContextRecording) { a.ObservationIDs = nil }},
		{"unknown_source", func(_ *auditTools, a *prContextRecording) { a.ObservationIDs = append(a.ObservationIDs, "invented") }},
		{"duplicate_context_id", func(_ *auditTools, a *prContextRecording) {
			a.PRContext.BeforeObservationIDs = append(a.PRContext.BeforeObservationIDs, a.PRContext.BeforeObservationIDs[0])
		}},
		{"input_oversize", func(_ *auditTools, a *prContextRecording) { a.Claim = strings.Repeat("x", 8001) }},
		{"merged_oversize", func(tools *auditTools, a *prContextRecording) {
			v := tools.ledger[a.ID]
			v.Evidence = []string{strings.Repeat("x", 7000)}
			tools.ledger[a.ID] = v
		}},
		{"verification_stage", func(tools *auditTools, _ *prContextRecording) { tools.trace[0].Stage = "verification" }},
		{"duplicate_trace", func(tools *auditTools, _ *prContextRecording) { tools.trace = append(tools.trace, tools.trace[0]) }},
		{"failed_source", func(tools *auditTools, _ *prContextRecording) { tools.trace[0].Error = "source unavailable" }},
		{"stale_source", func(tools *auditTools, _ *prContextRecording) {
			var o toolOutput
			json.Unmarshal([]byte(tools.trace[0].Output), &o)
			o.HeadSHA = "stale"
			b, _ := json.Marshal(o)
			tools.trace[0].Output = string(b)
		}},
		{"noneligible", func(tools *auditTools, _ *prContextRecording) {
			var o toolOutput
			json.Unmarshal([]byte(tools.trace[0].Output), &o)
			o.EvidenceEligible = false
			b, _ := json.Marshal(o)
			tools.trace[0].Output = string(b)
		}},
		{"output_id_mismatch", func(tools *auditTools, _ *prContextRecording) {
			var o toolOutput
			json.Unmarshal([]byte(tools.trace[0].Output), &o)
			o.ObservationID = "other"
			b, _ := json.Marshal(o)
			tools.trace[0].Output = string(b)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools, a, _ := contextRecordingFixture(t)
			tc.mutate(tools, &a)
			before, _ := json.Marshal(tools.ledger)
			out, _ := tools.recordPRContext(context.Background(), a)
			after, _ := json.Marshal(tools.ledger)
			if out.Error == "" || string(before) != string(after) || out.EvidenceEligible {
				t.Fatal(out, string(before), string(after))
			}
		})
	}
}

func TestRecordPRContextExplicitCorrectionKeepsTraceAndFamily(t *testing.T) {
	tools, a, _ := contextRecordingFixture(t)
	wrong := a
	wrong.PRContext = clonePRContext(a.PRContext)
	wrong.PRContext.BeforeObservationIDs = a.PRContext.AfterObservationIDs
	failed, _ := tools.recordPRContext(context.Background(), wrong)
	corrected, _ := tools.recordPRContext(context.Background(), a)
	if failed.Error == "" || corrected.Error != "" {
		t.Fatal(failed, corrected)
	}
	// A context patch cannot retire an update_investigation error, even at the same ID/claim.
	update := ToolTrace{Name: "update_investigation", Arguments: tools.trace[len(tools.trace)-1].Arguments}
	if sameRecordingArtifact(update, tools.trace[len(tools.trace)-1]) {
		t.Fatal("cross-family correction accepted")
	}
	result, _ := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: []recordingCorrection{{FailedObservationID: failed.ObservationID, CorrectedObservationID: corrected.ObservationID}}})
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if tools.trace[3].Error == "" {
		t.Fatal("failure history erased")
	}
}

func TestRecordPRContextConcurrentSourceMergesDoNotLoseLinks(t *testing.T) {
	tools, a, _ := contextRecordingFixture(t)
	patches := []prContextRecording{a, a}
	patches[0].PRContext = clonePRContext(a.PRContext)
	patches[0].PRContext.Relationships = nil
	patches[0].ObservationIDs = a.ObservationIDs[:2]
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, patch := range patches {
		wg.Add(1)
		go func(p prContextRecording) {
			defer wg.Done()
			<-start
			out, _ := tools.recordPRContext(context.Background(), p)
			if out.Error != "" {
				t.Error(out.Error)
			}
		}(patch)
	}
	close(start)
	wg.Wait()
	got := tools.investigations()[0]
	for _, id := range a.ObservationIDs {
		if !slices.Contains(got.ObservationIDs, id) {
			t.Fatal("source merge lost a concurrent link", got)
		}
	}
	if got.Claim != a.Claim || got.Status != "supported" || len(got.Plan) != 4 {
		t.Fatal("concurrent patch overwrote unrelated fields", got)
	}
}
