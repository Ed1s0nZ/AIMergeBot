package platform

import (
	"encoding/json"
	"testing"
)

func supportedChecks(id string) []VerificationCheck {
	out := []VerificationCheck{}
	for _, kind := range verificationKinds {
		out = append(out, VerificationCheck{Kind: kind, Status: "supported", Reason: "Conditional source fact", ObservationIDs: []string{id}})
	}
	return out
}
func TestVerificationChecksRequireCompleteFreshLinkedEvidence(t *testing.T) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	f := Finding{Side: "head", File: "entry.any", Line: 1, Evidence: "debit(amount)"}
	raw, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: "head", Text: "1: debit(amount)"})
	trace := []ToolTrace{{Stage: "verification", Name: "read_file", ObservationID: "fresh", Arguments: `{"path":"entry.any"}`, Output: string(raw)}}
	base := verificationInput{Status: "supported", ClaimCoverage: "full", Reason: "conditional outcome", Limitations: []string{}, ObservationIDs: []string{"fresh"}, Checks: supportedChecks("fresh")}
	for _, tc := range []struct {
		name    string
		edit    func(*verificationInput)
		wantErr bool
		status  string
	}{
		{"complete", func(v *verificationInput) {}, false, "supported"},
		{"missing", func(v *verificationInput) { v.Checks = nil }, false, "inconclusive"},
		{"missing outcome", func(v *verificationInput) { v.Checks = v.Checks[:3] }, false, "inconclusive"},
		{"uncertain guard", func(v *verificationInput) { v.Checks[2].Status = "inconclusive" }, false, "inconclusive"},
		{"contradicted outcome", func(v *verificationInput) { v.Checks[3].Status = "rejected" }, false, "inconclusive"},
		{"duplicate kind", func(v *verificationInput) { v.Checks[3].Kind = "guards" }, true, ""},
		{"unknown kind", func(v *verificationInput) { v.Checks[0].Kind = "runtime_proof" }, true, ""},
		{"unlinked source", func(v *verificationInput) { v.Checks[0].ObservationIDs = []string{"old"} }, true, ""},
		{"linked but stale", func(v *verificationInput) {
			v.ObservationIDs = append(v.ObservationIDs, "old")
			v.Checks[0].ObservationIDs = []string{"old"}
		}, true, ""},
		{"duplicate source", func(v *verificationInput) { v.Checks[0].ObservationIDs = []string{"fresh", "fresh"} }, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			in.Checks = cloneVerificationChecks(base.Checks)
			tc.edit(&in)
			got, err := validateVerification(in, snap, f, trace)
			if (err != nil) != tc.wantErr {
				t.Fatal(got, err)
			}
			if err == nil && got.Status != tc.status {
				t.Fatal(got)
			}
			if err == nil && len(got.Checks) > 0 {
				got.Checks[0].ObservationIDs[0] = "changed"
				if in.Checks[0].ObservationIDs[0] == "changed" {
					t.Fatal("alias")
				}
			}
		})
	}
}
func TestVerificationChecksStrictParsingAndHistoricalRead(t *testing.T) {
	in := verificationInput{Status: "supported", ClaimCoverage: "full", Reason: "conditional", Limitations: []string{}, ObservationIDs: []string{"fresh"}, Checks: supportedChecks("fresh")}
	raw, _ := json.Marshal(in)
	if _, err := parseVerification(string(raw)); err != nil {
		t.Fatal(err)
	}
	in.Checks[0].Reason = ""
	raw, _ = json.Marshal(in)
	if _, err := parseVerification(string(raw)); err == nil {
		t.Fatal("empty reason")
	}
	var historical FindingVerification
	if err := json.Unmarshal([]byte(`{"status":"supported","claim_coverage":"full","reason":"old"}`), &historical); err != nil || historical.Status != "supported" || historical.Checks != nil {
		t.Fatal("rewrote historical result", historical, err)
	}
}
