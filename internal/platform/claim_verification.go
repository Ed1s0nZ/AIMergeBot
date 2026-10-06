package platform

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ClaimVerification compares two static judgments of the same actual claim.
// Only the independent server runner may populate it; consistency is not proof.
type ClaimVerification struct {
	Status         string   `json:"status"`
	Verdict        string   `json:"verdict,omitempty"`
	AssessedClaim  string   `json:"assessed_claim"`
	Model          string   `json:"model,omitempty"`
	Reason         string   `json:"reason"`
	Limitations    []string `json:"limitations"`
	ObservationIDs []string `json:"observation_ids"`
	BaseSHA        string   `json:"base_sha"`
	HeadSHA        string   `json:"head_sha"`
}

type claimVerificationInput struct {
	Verdict        string   `json:"verdict"`
	Reason         string   `json:"reason"`
	Limitations    []string `json:"limitations"`
	ObservationIDs []string `json:"observation_ids"`
}

func parseClaimVerification(raw string) (claimVerificationInput, error) {
	var input claimVerificationInput
	if len(raw) > 16*1024 {
		return input, fmt.Errorf("claim verification response exceeds budget")
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return input, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return input, fmt.Errorf("trailing claim verification data")
	}
	if input.Verdict != "true" && input.Verdict != "false" && input.Verdict != "unknown" {
		return input, fmt.Errorf("invalid claim verdict")
	}
	if !boundedFact(input.Reason, 1000) || input.Limitations == nil || input.ObservationIDs == nil || len(input.Limitations) > 8 || len(input.ObservationIDs) > 20 {
		return input, fmt.Errorf("invalid claim verification explanation")
	}
	for _, limitation := range input.Limitations {
		if !boundedFact(limitation, 240) {
			return input, fmt.Errorf("invalid claim verification limitation")
		}
	}
	seen := map[string]bool{}
	for _, id := range input.ObservationIDs {
		if !boundedFact(id, 160) || seen[id] {
			return input, fmt.Errorf("invalid claim verification observation ID")
		}
		seen[id] = true
	}
	return input, nil
}

// Called only after fresh provenance and necessary-context checks, never on
// primary model submissions. No caller may infer safety from this comparison.
func claimJudgmentComparison(primary, verdict string) string {
	if verdict == "unknown" || (primary != "supported" && primary != "rejected") {
		return "inconclusive"
	}
	if verdict != "true" && verdict != "false" {
		return "inconclusive"
	}
	if (primary == "supported") == (verdict == "true") {
		return "consistent"
	}
	return "disagreed"
}

func cloneClaimVerification(v *ClaimVerification) *ClaimVerification {
	if v == nil {
		return nil
	}
	out := *v
	out.Limitations = append([]string{}, v.Limitations...)
	out.ObservationIDs = append([]string{}, v.ObservationIDs...)
	return &out
}
