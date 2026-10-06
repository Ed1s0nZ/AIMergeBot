package platform

import "fmt"

// Checks retain separate static model judgments; even four supported checks
// establish recording completeness only, not semantic or runtime correctness.
type VerificationCheck struct {
	Kind           string   `json:"kind"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason"`
	ObservationIDs []string `json:"observation_ids"`
}

var verificationKinds = []string{"input_control", "pr_causality", "guards", "outcome"}

func validateVerificationChecks(input verificationInput) error {
	if len(input.Checks) > 4 {
		return fmt.Errorf("too many verification checks")
	}
	allowed := map[string]bool{}
	for _, id := range input.ObservationIDs {
		allowed[id] = true
	}
	seen := map[string]bool{}
	for _, check := range input.Checks {
		known := false
		for _, kind := range verificationKinds {
			known = known || check.Kind == kind
		}
		if !known || seen[check.Kind] || !boundedFact(check.Reason, 400) || (check.Status != "supported" && check.Status != "rejected" && check.Status != "inconclusive") || len(check.ObservationIDs) < 1 || len(check.ObservationIDs) > 8 {
			return fmt.Errorf("invalid verification check")
		}
		seen[check.Kind] = true
		ids := map[string]bool{}
		for _, id := range check.ObservationIDs {
			if !allowed[id] || ids[id] {
				return fmt.Errorf("verification check must cite unique linked fresh sources")
			}
			ids[id] = true
		}
	}
	return nil
}
func cloneVerificationChecks(checks []VerificationCheck) []VerificationCheck {
	if checks == nil {
		return nil
	}
	out := append([]VerificationCheck{}, checks...)
	for i := range out {
		out[i].ObservationIDs = append([]string{}, out[i].ObservationIDs...)
	}
	return out
}
func gateVerificationChecks(v *FindingVerification) {
	if v.Status != "supported" {
		return
	}
	supported := map[string]bool{}
	for _, check := range v.Checks {
		supported[check.Kind] = check.Status == "supported"
	}
	for _, kind := range verificationKinds {
		if !supported[kind] {
			v.Status = "inconclusive"
			note := "Independent verification lacks supported source-linked checks for all four required aspects"
			v.Limitations = append(v.Limitations, note)
			reason := []rune(note + ". Unconfirmed model explanation: " + v.Reason)
			if len(reason) > 1000 {
				reason = append(reason[:999], '…')
			}
			v.Reason = string(reason)
			return
		}
	}
}
