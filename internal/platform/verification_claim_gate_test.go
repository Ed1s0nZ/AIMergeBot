package platform

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVerificationWholeClaimCoverage(t *testing.T) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	f := Finding{Side: "head", File: "entry.any", Line: 1, Evidence: "debit(amount)"}
	raw, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: "head", Text: "1: debit(amount)"})
	trace := []ToolTrace{{Stage: "verification", Name: "read_file", ObservationID: "fresh", Arguments: `{"path":"entry.any"}`, Output: string(raw)}}
	for _, coverage := range []string{"full", "partial", "unknown", ""} {
		input := verificationInput{Checks: supportedChecks("fresh"), Status: "supported", ClaimCoverage: coverage, Reason: strings.Repeat("界", 1000), Limitations: []string{"No evidence of goods delivery"}, ObservationIDs: []string{"fresh"}}
		v, err := validateVerification(input, snap, f, trace)
		if err != nil {
			t.Fatal(err)
		}
		if coverage == "full" {
			if v.Status != "supported" {
				t.Fatal(v)
			}
		} else if v.Status != "inconclusive" || len(v.Limitations) != 2 || v.Limitations[0] != input.Limitations[0] || utf8.RuneCountInString(v.Reason) > 1000 || !strings.Contains(v.Reason, "not accepted") {
			t.Fatal("partial or absent coverage certified the whole claim", v)
		}
		input.Status = "rejected"
		v, err = validateVerification(input, snap, f, trace)
		if err != nil || v.Status != "rejected" || len(v.Limitations) != 1 {
			t.Fatal("rejection changed", v, err)
		}
	}
	for _, coverage := range []string{"full", "partial"} {
		input := verificationInput{Status: "supported", ClaimCoverage: coverage, ObservationIDs: []string{"old"}}
		if _, err := validateVerification(input, snap, f, trace); err == nil {
			t.Fatal("coverage bypassed evidence provenance")
		}
	}
}

func TestVerificationCoverageParser(t *testing.T) {
	for _, coverage := range []string{"full", "partial", "unknown", ""} {
		raw, _ := json.Marshal(verificationInput{Status: "supported", ClaimCoverage: coverage, Reason: "Conditional risk only", Limitations: []string{}, ObservationIDs: []string{}})
		if _, err := parseVerification(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := parseVerification(`{"status":"supported","claim_coverage":"runtime_verified","reason":"bad","limitations":[],"observation_ids":[]}`); err == nil {
		t.Fatal("invalid coverage accepted")
	}
}
