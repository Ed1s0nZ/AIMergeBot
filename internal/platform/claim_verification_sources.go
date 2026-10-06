package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

const claimVerificationStage = "investigation_verification"

// This accepts only a fresh runner's registry, never the primary trace. Source
// provenance and snapshot coverage do not establish the truth of a code claim.
func validateClaimVerification(input claimVerificationInput, item Investigation, fresh *auditTools) (*ClaimVerification, error) {
	fresh.mu.Lock()
	trace := append([]ToolTrace{}, fresh.trace...)
	fresh.mu.Unlock()
	if fresh.stage != claimVerificationStage || fresh.observationPrefix == "" {
		return nil, fmt.Errorf("invalid claim verification context")
	}
	registry := map[string]ToolTrace{}
	counts := map[string]int{}
	for _, tr := range trace {
		registry[tr.ObservationID] = tr
		counts[tr.ObservationID]++
	}
	base, head, changed := false, false, false
	inspected := map[int]bool{}
	seen := map[string]bool{}
	for _, id := range input.ObservationIDs {
		tr, ok := registry[id]
		if !ok || counts[id] != 1 || seen[id] || !strings.HasPrefix(id, fresh.observationPrefix+"-") || tr.Stage != claimVerificationStage || tr.Error != "" || tr.Partial || !isSourceTool(tr.Name) {
			return nil, fmt.Errorf("claim verifier observation is not unique fresh successful source")
		}
		seen[id] = true
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || out.ObservationID != id || !out.EvidenceEligible || out.Error != "" || out.More || !observationAtSnapshot(out, fresh.snap) || strings.TrimSpace(out.Text) == "" {
			return nil, fmt.Errorf("claim verifier source is not eligible at fixed snapshot")
		}
		if out.RepositoryID != 0 {
			inspected[out.RepositoryID] = true
			continue
		}
		b, h, c := claimPrimaryCoverage(tr, out, fresh.scope)
		base, head, changed = base || b, head || h, changed || c
	}
	v := &ClaimVerification{Status: claimJudgmentComparison(item.Status, input.Verdict), Verdict: input.Verdict, AssessedClaim: item.Claim, Reason: input.Reason, Limitations: append([]string{}, input.Limitations...), ObservationIDs: append([]string{}, input.ObservationIDs...), BaseSHA: fresh.snap.BaseSHA, HeadSHA: fresh.snap.HeadSHA}
	if input.Verdict == "unknown" {
		return v, nil
	}
	missing := !base || !head || !changed
	for _, source := range contextPolicyItems(fresh.snap) {
		missing = missing || !inspected[source.ProjectID]
	}
	if missing {
		v.Status, v.Verdict = "inconclusive", "unknown"
		note := "复核未引用完整的主PR双侧变更来源或必要的固定上下文，模型命题判断未获接纳。"
		if len(v.Limitations) >= 8 {
			v.Limitations = v.Limitations[:7]
		}
		v.Limitations = append(v.Limitations, note)
		v.Reason = boundedClaimReason(note + " 未确认的模型解释：" + v.Reason)
	}
	return v, nil
}

func boundedClaimReason(reason string) string {
	runes := []rune(reason)
	if len(runes) > 1000 {
		return string(runes[:999]) + "…"
	}
	return reason
}

// Read sources identify snapshot sides; only included paths establish the PR
// relation. Canonical metadata-only entries carry both fixed Git facts without
// pretending to inspect an execution path. History alone is not a side read.
func claimPrimaryCoverage(tr ToolTrace, out toolOutput, scope DiffScope) (base, head, changed bool) {
	included := func(path string) bool {
		for _, p := range scope.Included {
			if path == p {
				return true
			}
		}
		_, old := scope.Removed[path]
		return old
	}
	switch tr.Name {
	case "read_file":
		var a readArgs
		if json.Unmarshal([]byte(tr.Arguments), &a) == nil {
			c := included(a.Path)
			return a.Base && c, !a.Base && c, c
		}
	case "read_files":
		var a batchArgs
		if json.Unmarshal([]byte(tr.Arguments), &a) == nil {
			for _, f := range a.Files {
				c := included(f.Path)
				base, head, changed = base || f.Base && c, head || !f.Base && c, changed || c
			}
		}
	case "get_diff", "compare_files":
		var a struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(tr.Arguments), &a) == nil && included(a.Path) {
			return true, true, true
		}
	case "get_change_metadata":
		var a metadataArgs
		if json.Unmarshal([]byte(tr.Arguments), &a) == nil {
			expected, ok := scope.Metadata[a.Path]
			if ok && expected.valid() && expected.metadataOnly() && out.Metadata != nil && out.Metadata.canonical() == expected.canonical() && out.Text == expected.canonical() {
				return true, true, true
			}
		}
	}
	return
}
