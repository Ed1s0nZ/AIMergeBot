package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSequenceContextSelectsPinnedEvidenceAndCounterevidence(t *testing.T) {
	tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}, ledger: map[string]Investigation{"i": {ObservationIDs: []string{"evidence"}, CounterObservationIDs: []string{"counter"}}}}
	add := func(id, name, path, text, head string) {
		raw, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: head, Text: text})
		args, _ := json.Marshal(map[string]string{"path": path})
		tools.trace = append(tools.trace, ToolTrace{ObservationID: id, Name: name, Arguments: string(args), Output: string(raw)})
	}
	add("unrelated", "read_file", "other.any", "UNRELATED", "head")
	add("process", "submit_finding", "target.any", "PROCESS", "head")
	add("counter", "read_file", "guard.any", "COUNTER", "head")
	add("wrong", "read_file", "target.any", "WRONG_SNAPSHOT", "latest")
	add("relevant", "read_file", "target.any", "RELEVANT", "head")
	add("evidence", "read_file", "target.any", "PRIMARY", "head")
	f := Finding{File: "target.any", InvestigationID: "i", ObservationIDs: []string{"evidence", "process", "wrong"}}
	out := tools.sequenceObservations(f)
	for _, excluded := range []string{"UNRELATED", "PROCESS", "WRONG_SNAPSHOT"} {
		if strings.Contains(out, excluded) {
			t.Fatalf("included %s: %s", excluded, out)
		}
	}
	for _, included := range []string{"PRIMARY", "COUNTER", "RELEVANT"} {
		if !strings.Contains(out, included) {
			t.Fatalf("missing %s", included)
		}
	}
	if strings.Index(out, "PRIMARY") > strings.Index(out, "COUNTER") || strings.Index(out, "COUNTER") > strings.Index(out, "RELEVANT") {
		t.Fatal("evidence priority lost")
	}
	add("large", "read_file", "target.any", strings.Repeat("x", 40000), "head")
	out = tools.sequenceObservations(f)
	if len(out) > 32*1024 || !strings.Contains(out, `"related_observations_omitted":true`) {
		t.Fatal("context budget/omission contract failed")
	}
	var v any
	if json.Unmarshal([]byte(out), &v) != nil {
		t.Fatal("truncated JSON")
	}
}

func TestSequenceScheduleStableWithoutChangingOutputOrder(t *testing.T) {
	findings := []Finding{{ID: "low", Severity: "low", Confidence: "supported"}, {ID: "candidate", Severity: "high", Confidence: "candidate"}, {ID: "supported", Severity: "high", Confidence: "supported"}}
	order := sequenceOrder(findings)
	if order[0] != 2 || order[1] != 1 || order[2] != 0 || findings[0].ID != "low" {
		t.Fatal(order, findings)
	}
}

func TestSupplementalCheckpointRetainsFinalPrimaryAndCompletedDiagram(t *testing.T) {
	var checkpoints []AuditResult
	tools := &auditTools{progress: func(r AuditResult, _ []ToolTrace) error { checkpoints = append(checkpoints, r); return nil }}
	result := AuditResult{Findings: []Finding{{ID: "f", SequenceDiagram: &SequenceDiagram{Status: "partial"}}}, Summary: "primary summary", CoverageNotes: []string{"primary limitation"}, ExcludedFiles: []string{"docs.md"}}
	tools.sequenceCheckpoint(result)
	result.Findings[0].SequenceDiagram.Status = "unavailable"
	// A subsequent graph tool callback must keep the frozen completed diagram.
	tools.checkpoint()
	got := checkpoints[len(checkpoints)-1]
	if got.Summary != "primary summary" || got.Findings[0].SequenceDiagram.Status != "partial" || len(got.ExcludedFiles) != 1 || got.CoverageNotes[0] != "primary limitation" {
		t.Fatalf("checkpoint overwritten: %+v", got)
	}
	tools.sequenceCheckpoint(result)
	if checkpoints[len(checkpoints)-1].Findings[0].SequenceDiagram.Status != "unavailable" {
		t.Fatal("new graph result not checkpointed")
	}
}
