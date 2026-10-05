package platform

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func sarifFixture() Run {
	return Run{ID: 7, Snapshot: Snapshot{ProjectID: 1, SourceProjectID: 3, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("c", 40)}}}}, PolicyVersion: PolicyVersion, Status: "incomplete", Result: AuditResult{CoverageNotes: []string{"Caller authentication remains unknown"}, Findings: []Finding{
		{ID: "removed", Side: "base", File: "src/订单 check.any", Line: 4, Type: "authorization", Title: "Guard removal", Severity: "high", Evidence: "authorize(user)", Fingerprint: "existing-fingerprint", PRContext: &PRInvestigationContext{UnresolvedEdges: []string{"cross-service contract not established"}}, SequenceDiagram: &SequenceDiagram{Status: "partial", SequenceInput: SequenceInput{Steps: []SequenceStep{{Certainty: "inferred", Evidence: []SequenceReference{{RepositoryID: 2, Side: "head", File: "api.any", Line: 2, SHA: strings.Repeat("c", 40), Snippet: "validateActor()"}, {RepositoryID: 99, Side: "head", File: "private.any", Line: 2, Snippet: "unavailable"}}}}}}},
		{ID: "metadata", Side: "head", File: "entry.any", AnchorType: "git_metadata", Type: "file mode", Line: 0, Severity: "low", Title: "Executable mode"},
	}}}
}
func TestSARIFPreservesSnapshotIdentityAndUncertainty(t *testing.T) {
	report := BuildSARIF(sarifFixture(), []Review{{FindingID: "removed", Status: "false_positive", Reason: "Guard exists elsewhere", Revision: 1}})
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "codeFlows") || strings.Contains(string(raw), "private.any") {
		t.Fatal("invented flow or unmapped evidence")
	}
	run := report["runs"].([]sarifObject)[0]
	results := run["results"].([]sarifObject)
	location := results[0]["locations"].([]sarifObject)[0]["physicalLocation"].(sarifObject)
	artifact := location["artifactLocation"].(sarifObject)
	if artifact["uriBaseId"] != "BASE" || !strings.Contains(artifact["uri"].(string), "%20") || location["region"].(sarifObject)["startLine"] != 4 {
		t.Fatal("BASE location lost", location)
	}
	metadata := results[1]["locations"].([]sarifObject)[0]["physicalLocation"].(sarifObject)
	if _, ok := metadata["region"]; ok {
		t.Fatal("line zero metadata became source line")
	}
	related := results[0]["relatedLocations"].([]sarifObject)
	if len(related) != 1 || related[0]["physicalLocation"].(sarifObject)["artifactLocation"].(sarifObject)["uriBaseId"] != "CONTEXT_2" {
		t.Fatal("cross-repository citation lost")
	}
	if run["invocations"].([]sarifObject)[0]["executionSuccessful"] != false {
		t.Fatal("incomplete marked complete")
	}
	if _, ok := results[0]["suppressions"]; !ok {
		t.Fatal("human review lost")
	}
	if output := os.Getenv("AIMB_SARIF_FIXTURE_OUTPUT"); output != "" {
		if err = os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestSARIFEmptyAndHistoricalFindings(t *testing.T) {
	report := BuildSARIF(Run{Status: "succeeded", Result: AuditResult{Findings: []Finding{{ID: "old", File: "old.any", Line: 1}}}}, nil)
	results := report["runs"].([]sarifObject)[0]["results"].([]sarifObject)
	if results[0]["properties"].(sarifObject)["verificationStatus"] != "unverified" {
		t.Fatal("historic finding invented verification")
	}
	empty := BuildSARIF(Run{Status: "incomplete"}, nil)
	if empty["runs"].([]sarifObject)[0]["results"].([]sarifObject) == nil {
		t.Fatal("empty results serialized null")
	}
}
