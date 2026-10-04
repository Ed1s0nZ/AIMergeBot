package platform

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestSupportedFindingRequiresRealLinkedSourceObservation(t *testing.T) {
	repo, snap, f, diagramInput := sequenceFixture()
	tools := &auditTools{repo: repo, snap: snap, cache: map[string]string{}, scope: DiffScope{Added: map[string]map[int]bool{f.File: {f.Line: true}}}}
	ctx := context.Background()
	f.Confidence = "supported"
	if _, err := tools.acceptFinding(ctx, f); err == nil {
		t.Fatal("unlinked supported finding accepted")
	}
	source, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 2})
	if source.Error != "" {
		t.Fatal(source.Error)
	}
	item := Investigation{ID: "source-path", Claim: "input reaches sink", Evidence: []string{"static code candidate"}, ObservationIDs: []string{source.ObservationID}}
	record, _ := tools.record(ctx, item)
	if record.Error != "" {
		t.Fatal(record.Error)
	}
	item.Status = "supported"
	updated, _ := tools.update(ctx, item)
	if updated.Error != "" {
		t.Fatal(updated.Error)
	}
	f.InvestigationID = item.ID
	f.ObservationIDs = []string{source.ObservationID}
	if _, err := tools.acceptFinding(ctx, f); err != nil {
		t.Fatal(err)
	}
	f.ObservationIDs = []string{"observation-invented"}
	if _, err := tools.acceptFinding(ctx, f); err == nil {
		t.Fatal("invented observation accepted")
	}
	item.ObservationIDs = []string{record.ObservationID}
	bad, _ := tools.update(ctx, item)
	if bad.Error == "" {
		t.Fatal("process output used as source evidence")
	}
	if path := os.Getenv("AIM_HARDENING_PROOF"); path != "" {
		result := AuditResult{Findings: tools.acceptedFindings(), Investigations: tools.investigations(), Summary: "Deterministic source-provenance fixture; not a runtime exploit", CoverageNotes: []string{}}
		diagram, err := ValidateSequence(ctx, repo, snap, result.Findings[0], diagramInput)
		if err != nil {
			t.Fatal(err)
		}
		result.Findings[0].SequenceDiagram = diagram
		raw, _ := json.Marshal(Run{Snapshot: snap, Status: "succeeded", Result: result, Trace: tools.trace})
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}

}
