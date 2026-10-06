package platform

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestVerifierObservationDiagnosticsPreserveRejectionAndPrivacy(t *testing.T) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	finding := Finding{Side: "head", File: "source.any", Line: 1, Evidence: "sensitive(input)"}
	id := "PRIVATE REJECTED ID"
	output, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: "head", Text: "1: sensitive(input)"})
	good := ToolTrace{Stage: "verification", Name: "read_file", ObservationID: id, Arguments: `{"path":"source.any"}`, Output: string(output)}
	input := verificationInput{Status: "supported", ClaimCoverage: "full", Reason: "fixture", Limitations: []string{}, ObservationIDs: []string{id}, Checks: supportedChecks(id)}
	for _, code := range []string{"unknown", "wrong_stage", "non_source", "read_failed", "malformed_output", "snapshot_mismatch", "empty_source"} {
		t.Run(code, func(t *testing.T) {
			bad := good
			trace := []ToolTrace{bad}
			switch code {
			case "unknown":
				trace = nil
			case "wrong_stage":
				bad.Stage = "primary"
			case "non_source":
				bad.Name = "list_files"
			case "read_failed":
				bad.Error = "PRIVATE IO FAILURE"
			case "malformed_output":
				bad.Output = "PRIVATE MALFORMED SOURCE"
			case "snapshot_mismatch":
				bad.Output = `{"base_sha":"base","head_sha":"moving","text":"PRIVATE SOURCE"}`
			case "empty_source":
				bad.Output = `{"base_sha":"base","head_sha":"head","text":" "}`
			}
			if code != "unknown" {
				trace[0] = bad
			}
			_, err := validateVerification(input, snap, finding, trace)
			if err == nil || err.Error() != "verifier observation is not a fresh successful pinned source" {
				t.Fatal("rejection changed", err)
			}
			diagnosticCode := verificationObservationFailureCode(err)
			if diagnosticCode != "invalid_observation_"+code {
				t.Fatal("wrong failure classification", diagnosticCode)
			}
			diagnostic := responseDiagnostic(`{"reason":"PRIVATE MODEL TEXT"}`, responseError(diagnosticCode))
			if strings.Contains(diagnostic, "PRIVATE") || len(diagnostic) > 300 {
				t.Fatal("private payload in diagnostic", diagnostic)
			}
		})
	}
	if _, err := validateVerification(input, snap, finding, []ToolTrace{good}); err != nil {
		t.Fatal("valid source blocked", err)
	}
	if verificationObservationFailureCode(&verifierObservationError{cause: "PRIVATE MODEL TEXT"}) != "invalid_observation" || verificationObservationFailureCode(errors.New("PRIVATE IO FAILURE")) != "invalid_observation" {
		t.Fatal("untrusted cause leaked")
	}
}
