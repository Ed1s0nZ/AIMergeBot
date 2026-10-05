package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestContextSequenceReferencesUseActualFixedRepositoryAndCannotReplaceRiskAnchor(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	input := SequenceInput{Participants: []SequenceParticipant{{ID: "client", Label: "Caller"}, {ID: "service", Label: "Primary service"}}, Steps: []SequenceStep{
		{From: "client", To: "service", Label: "Primary sensitive operation", Kind: "call", Certainty: "cited", Risk: true, Evidence: []SequenceReference{{Side: "head", File: f.File, Line: f.Line, Snippet: f.Evidence}}},
		{From: "client", To: "service", Label: "Related source text, relation remains conditional", Kind: "note", Certainty: "inferred", Evidence: []SequenceReference{{RepositoryID: 2, Side: "head", File: f.File, Line: 3, Snippet: "context-only needle"}}},
	}, Limitations: []string{"Static context does not prove a runtime cross-service call"}}
	diagram, err := ValidateSequence(context.Background(), root, snap, f, input, sources)
	if err != nil || diagram.Status != "partial" || diagram.Steps[1].Evidence[0].SHA != sources[2].Snapshot.HeadSHA || !strings.Contains(diagram.Mermaid, "仓库") {
		t.Fatal("related citation validation", err)
	}
	clone := func() SequenceInput {
		raw, _ := json.Marshal(input)
		var copy SequenceInput
		json.Unmarshal(raw, &copy)
		return copy
	}
	wrong := clone()
	wrong.Steps[1].Evidence[0].SHA = snap.HeadSHA
	if _, err = ValidateSequence(context.Background(), root, snap, f, wrong, sources); err == nil {
		t.Fatal("cross-repository SHA mismatch accepted")
	}
	unsupported := clone()
	unsupported.Steps[0].Evidence[0].RepositoryID = 2
	if _, err = ValidateSequence(context.Background(), root, snap, f, unsupported, sources); err == nil {
		t.Fatal("identical related snippet replaced primary risk anchor")
	}
	unknown := clone()
	unknown.Steps[1].Evidence[0].RepositoryID = 999
	if _, err = ValidateSequence(context.Background(), root, snap, f, unknown, sources); err == nil {
		t.Fatal("arbitrary diagram repository accepted")
	}
	denied := sources[2]
	denied.Authorize = func(context.Context) error { return ErrContextRepository }
	sources[2] = denied
	if _, err = ValidateSequence(context.Background(), root, snap, f, clone(), sources); err == nil {
		t.Fatal("diagram citation bypassed revocation")
	}
}

func TestContextSequenceObservationsIncludeLinkedSourcesButNotUnrelatedSameName(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	ctx := context.Background()
	primary, _ := tools.file(ctx, readArgs{Path: f.File, Start: 2, End: 2})
	extra, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: f.File, Start: 3, End: 3})
	f.ObservationIDs = []string{primary.ObservationID}
	text := tools.sequenceObservations(f)
	if strings.Contains(text, extra.ObservationID) {
		t.Fatal("same filename implicitly established related evidence")
	}
	f.InvestigationID = "related"
	tools.ledger = map[string]Investigation{"related": {CounterObservationIDs: []string{extra.ObservationID}}}
	text = tools.sequenceObservations(f)
	if !strings.Contains(text, extra.ObservationID) || !strings.Contains(text, `"repository_id":2`) {
		t.Fatal("linked fixed counterevidence lost repository identity")
	}
}
