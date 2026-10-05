package platform

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type FindingVerification struct {
	Model          string   `json:"model,omitempty"`
	Status         string   `json:"status"`
	ClaimCoverage  string   `json:"claim_coverage,omitempty"`
	Reason         string   `json:"reason"`
	Limitations    []string `json:"limitations"`
	ObservationIDs []string `json:"observation_ids"`
	BaseSHA        string   `json:"base_sha"`
	HeadSHA        string   `json:"head_sha"`
}

type verificationInput struct {
	Status         string   `json:"status"`
	ClaimCoverage  string   `json:"claim_coverage,omitempty"`
	Reason         string   `json:"reason"`
	Limitations    []string `json:"limitations"`
	ObservationIDs []string `json:"observation_ids"`
}

func parseVerification(raw string) (verificationInput, error) {
	var input verificationInput
	if len(raw) > 16*1024 {
		return input, fmt.Errorf("verification response exceeds budget")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return input, fmt.Errorf("trailing verification data")
	}
	if input.Status != "supported" && input.Status != "rejected" && input.Status != "inconclusive" {
		return input, fmt.Errorf("invalid verification verdict")
	}
	if input.ClaimCoverage != "" && input.ClaimCoverage != "full" && input.ClaimCoverage != "partial" && input.ClaimCoverage != "unknown" {
		return input, fmt.Errorf("invalid verification claim coverage")
	}
	if strings.TrimSpace(input.Reason) == "" || utf8.RuneCountInString(input.Reason) > 1000 || input.Limitations == nil || input.ObservationIDs == nil || len(input.Limitations) > 8 || len(input.ObservationIDs) > 20 {
		return input, fmt.Errorf("invalid verification explanation")
	}
	for _, limitation := range input.Limitations {
		if strings.TrimSpace(limitation) == "" || utf8.RuneCountInString(limitation) > 240 {
			return input, fmt.Errorf("invalid verification limitation")
		}
	}
	return input, nil
}

// A verdict is accepted only against observations from this fresh verifier
// context. This checks provenance and anchor text, not runtime exploitability.
func validateVerification(input verificationInput, snap Snapshot, f Finding, trace []ToolTrace) (*FindingVerification, error) {
	seen := map[string]bool{}
	anchor := false
	inspected := map[int]bool{}
	for _, id := range input.ObservationIDs {
		if seen[id] {
			return nil, fmt.Errorf("duplicate verifier observation")
		}
		seen[id] = true
		found := false
		for _, tr := range trace {
			if tr.Stage != "verification" || tr.ObservationID != id || tr.Error != "" || !isSourceTool(tr.Name) {
				continue
			}
			var output toolOutput
			if json.Unmarshal([]byte(tr.Output), &output) != nil || !observationAtSnapshot(output, snap) || output.Error != "" || strings.TrimSpace(output.Text) == "" {
				continue
			}
			found = true
			if output.RepositoryID != 0 {
				inspected[output.RepositoryID] = true
				continue
			}
			if f.AnchorType == "git_metadata" {
				if tr.Name == "get_change_metadata" && output.Metadata != nil && f.Metadata != nil && output.Metadata.canonical() == f.Metadata.canonical() && output.Text == f.Evidence {
					anchor = true
				}
			} else if tr.Name == "read_file" {
				var args readArgs
				if json.Unmarshal([]byte(tr.Arguments), &args) == nil && args.Path == f.File && args.Base == (f.Side == "base") {
					prefix := fmt.Sprintf("%d: ", f.Line)
					for _, line := range strings.Split(output.Text, "\n") {
						if strings.HasPrefix(line, prefix) && strings.Contains(strings.TrimPrefix(line, prefix), f.Evidence) {
							anchor = true
						}
					}
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("verifier observation is not a fresh successful pinned source")
		}
	}
	if input.Status != "inconclusive" && len(input.ObservationIDs) == 0 {
		return nil, fmt.Errorf("verifier verdict requires fresh evidence")
	}
	if input.Status == "supported" && !anchor {
		return nil, fmt.Errorf("support verdict lacks freshly read primary anchor")
	}
	verified := &FindingVerification{Status: input.Status, ClaimCoverage: input.ClaimCoverage, Reason: input.Reason, Limitations: append([]string{}, input.Limitations...), ObservationIDs: append([]string{}, input.ObservationIDs...), BaseSHA: snap.BaseSHA, HeadSHA: snap.HeadSHA}
	gateContextVerification(verified, snap, inspected)
	gateClaimCoverage(verified)
	return verified, nil
}
