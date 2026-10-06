package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func recordingCorrectionFixture(t *testing.T) (*auditTools, toolOutput, toolOutput) {
	t.Helper()
	tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}}
	bad, err := tools.record(context.Background(), Investigation{ID: "inv", Claim: "conditional candidate", ObservationIDs: []string{"unknown"}})
	if err != nil || bad.Error == "" {
		t.Fatal("missing fixture rejection", bad, err)
	}
	good, err := tools.record(context.Background(), Investigation{ID: "inv", Claim: "conditional candidate"})
	if err != nil || good.Error != "" {
		t.Fatal("fixture record failed", good, err)
	}
	return tools, bad, good
}
func hasPendingObservation(tools *auditTools, id string) bool {
	for _, item := range tools.unresolved() {
		if item.ObservationID == id {
			return true
		}
	}
	return false
}
func TestRecordingCorrectionRetainsHistoryAndActualGaps(t *testing.T) {
	tools, bad, good := recordingCorrectionFixture(t)
	original := tools.trace[0]
	out, err := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: []recordingCorrection{{bad.ObservationID, good.ObservationID}}})
	if err != nil || out.Error != "" || out.EvidenceEligible || hasPendingObservation(tools, bad.ObservationID) {
		t.Fatal("valid correction did not retire pending record", out, err)
	}
	if tools.trace[0] != original || len(tools.ledger) != 1 || len(investigationPlanCoverage(tools.investigations())) == 0 {
		t.Fatal("history or actual plan gap erased")
	}
	registered, err := tools.register()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range registered {
		info, _ := entry.Info(context.Background())
		found = found || info.Name == "resolve_recording_errors"
	}
	if !found {
		t.Fatal("correction tool not registered")
	}
	for _, entry := range readOnlyTools(context.Background(), registered) {
		info, _ := entry.Info(context.Background())
		if info.Name == "resolve_recording_errors" {
			t.Fatal("verifier can clear primary pending work")
		}
	}
}
func TestRecordingCorrectionRejectsForeignAndIncompleteReferencesAtomically(t *testing.T) {
	for _, mode := range []string{"unknown", "reversed", "other_id", "other_claim", "stale", "other_stage", "source", "partial", "duplicate", "atomic"} {
		t.Run(mode, func(t *testing.T) {
			tools, bad, good := recordingCorrectionFixture(t)
			pairs := []recordingCorrection{{bad.ObservationID, good.ObservationID}}
			switch mode {
			case "unknown":
				pairs[0].CorrectedObservationID = "PRIVATE UNKNOWN ID"
			case "reversed":
				pairs[0] = recordingCorrection{good.ObservationID, bad.ObservationID}
			case "other_id":
				tools.trace[1].Arguments = `{"id":"other","claim":"unrelated"}`
			case "other_claim":
				tools.trace[1].Arguments = `{"id":"inv","claim":"different resolved claim"}`
			case "stale":
				var out toolOutput
				json.Unmarshal([]byte(tools.trace[1].Output), &out)
				out.HeadSHA = "moving"
				raw, _ := json.Marshal(out)
				tools.trace[1].Output = string(raw)
			case "other_stage":
				tools.trace[1].Stage = "verification"
			case "source":
				tools.trace[0].Name = "read_file"
			case "partial":
				tools.trace[1].Partial = true
			case "duplicate":
				pairs = append(pairs, pairs[0])
			case "atomic":
				pairs = append(pairs, recordingCorrection{"unknown", "unknown"})
			}
			out, err := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: pairs})
			if err != nil || out.Error == "" || !hasPendingObservation(tools, bad.ObservationID) {
				t.Fatal("invalid correction cleared work", out, err)
			}
			if strings.Contains(out.Error, "PRIVATE") || len(tools.unresolved()) != 1 {
				t.Fatal("error leaked reference or created recursive pending correction", out)
			}
		})
	}
}
func TestRecordingCorrectionCannotRetireSourceFailures(t *testing.T) {
	tools, bad, good := recordingCorrectionFixture(t)
	source, _ := tools.invoke("read_file", readArgs{Path: "source.any"}, func() (toolOutput, error) { return toolOutput{}, errors.New("failed source") })
	out, _ := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: []recordingCorrection{{source.ObservationID, good.ObservationID}}})
	if out.Error == "" || !hasPendingObservation(tools, source.ObservationID) || !hasPendingObservation(tools, bad.ObservationID) {
		t.Fatal("source failure erased", out)
	}
}
func TestRecordingCorrectionFindingRequiresSameDeclaredArtifact(t *testing.T) {
	a := Finding{ID: "candidate", InvestigationID: "inv", File: "service.any", Type: "authorization", Side: "head", Line: 9}
	b := a
	b.Side = "base"
	b.Line = 7
	rawA, _ := json.Marshal(a)
	rawB, _ := json.Marshal(b)
	first := ToolTrace{Name: "submit_finding", Arguments: string(rawA)}
	corrected := ToolTrace{Name: "submit_finding", Arguments: string(rawB)}
	if !sameRecordingArtifact(first, corrected) {
		t.Fatal("anchor correction rejected")
	}
	for _, field := range []string{"id", "investigation", "file", "type", "title", "description", "trigger"} {
		c := b
		switch field {
		case "id":
			c.ID = "different"
		case "investigation":
			c.InvestigationID = "different"
		case "file":
			c.File = "different"
		case "type":
			c.Type = "different"
		case "title":
			c.Title = "different"
		case "description":
			c.Description = "different"
		case "trigger":
			c.Trigger = "different"
		}
		raw, _ := json.Marshal(c)
		corrected.Arguments = string(raw)
		if sameRecordingArtifact(first, corrected) {
			t.Fatal("foreign candidate matched", field)
		}
	}
}

func TestRecordingCorrectionAfterValidatedFindingRetainsCandidateAndSourceGap(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	tools := &auditTools{repo: repo, snap: snap, scope: DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}}, cache: map[string]string{}}
	ctx := context.Background()
	source, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 2})
	record, _ := tools.record(ctx, Investigation{ID: "inv", Claim: "conditional risk", ObservationIDs: []string{source.ObservationID}})
	if record.Error != "" {
		t.Fatal(record)
	}
	f.ID = "candidate"
	f.InvestigationID = "inv"
	f.ObservationIDs = []string{source.ObservationID}
	badFinding := f
	badFinding.Side = "base"
	bad, _ := tools.submit(ctx, badFinding)
	if bad.Error == "" {
		t.Fatal("invalid side accepted")
	}
	good, _ := tools.submit(ctx, f)
	if good.Error != "" {
		t.Fatal(good)
	}
	out, _ := tools.resolveRecordingErrors(ctx, recordingCorrectionsArgs{Corrections: []recordingCorrection{{bad.ObservationID, good.ObservationID}}})
	if out.Error != "" || hasPendingObservation(tools, bad.ObservationID) {
		t.Fatal("validated finding correction failed", out)
	}
	accepted := tools.acceptedFindings()
	if len(accepted) != 1 || accepted[0].Side != "head" || len(findingPRCoverage(accepted)) == 0 {
		t.Fatal("candidate lost or actual PR gap hidden", accepted)
	}
	if tools.trace[2].Error == "" || tools.trace[3].Error != "" {
		t.Fatal("finding attempts rewritten")
	}
}
