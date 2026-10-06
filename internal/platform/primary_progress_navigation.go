package platform

import "encoding/json"

type primaryRecordingProgress struct {
	UninspectedContextIDs []int    `json:"uninspected_context_ids,omitempty"`
	LedgerCount           int      `json:"ledger_count"`
	RecordingGaps         []string `json:"recording_gaps,omitempty"`
	BaseSourceIDs         []string `json:"recent_primary_base_source_ids,omitempty"`
	HeadSourceIDs         []string `json:"recent_primary_head_source_ids,omitempty"`
}

// This projects runtime metadata only. Model claims, reasons, paths and source
// snippets must never be interpolated into the trusted system instruction.
func (t *auditTools) primaryProgressNavigation() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := primaryRecordingProgress{LedgerCount: len(t.ledger)}
	inspected := map[int]bool{}
	seenIDs := map[string]bool{}
	for i := len(t.trace) - 1; i >= 0; i-- {
		tr := t.trace[i]
		if (tr.Stage != "" && tr.Stage != "primary") || tr.Error != "" || !isSourceTool(tr.Name) {
			continue
		}
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || !out.EvidenceEligible || out.Error != "" || out.ObservationID == "" || len(out.ObservationID) > 80 || out.ObservationID != tr.ObservationID || !observationAtSnapshot(out, t.snap) {
			continue
		}
		if out.RepositoryID != 0 {
			inspected[out.RepositoryID] = true
			continue
		}
		if seenIDs[out.ObservationID] {
			continue
		}
		seenIDs[out.ObservationID] = true
		base, head := primarySourceSides(tr)
		if base && len(state.BaseSourceIDs) < 4 {
			state.BaseSourceIDs = append(state.BaseSourceIDs, out.ObservationID)
		}
		if head && len(state.HeadSourceIDs) < 4 {
			state.HeadSourceIDs = append(state.HeadSourceIDs, out.ObservationID)
		}
	}
	for _, source := range contextPolicyItems(t.snap) {
		if !inspected[source.ProjectID] && len(state.UninspectedContextIDs) < 8 {
			state.UninspectedContextIDs = append(state.UninspectedContextIDs, source.ProjectID)
		}
	}
	gaps := map[string]bool{}
	if len(t.ledger) == 0 {
		gaps["plan_missing"] = true
	}
	for _, item := range t.ledger {
		for _, gap := range investigationRecordingGaps(item) {
			gaps[gap] = true
		}
	}
	// Stable finite order, independent of map iteration or model text.
	for _, gap := range []string{"plan_missing", "plan_missing_core_aspects", "plan_unfinished", "pr_context_missing", "base_sources_missing", "head_sources_missing", "relationships_missing", "relationships_inferred"} {
		if gaps[gap] {
			state.RecordingGaps = append(state.RecordingGaps, gap)
		}
	}
	raw, _ := json.Marshal(state)
	return "\nServer-owned recording progress (navigation only, not source evidence): " + string(raw) + ". Uninspected configured context is not unavailable: use repository-scoped tools to check relevant contracts before judging compatibility. If ledger_count is zero, record a source-linked changed-behavior inspection with a plan, which may conclude rejected/no-risk; do not invent a finding. Recent source IDs are candidates for deliberate linkage, not proof of relevance or connection. Preserve actual unknowns."
}
func primarySourceSides(tr ToolTrace) (bool, bool) {
	switch tr.Name {
	case "get_diff", "compare_files", "get_change_metadata":
		return true, true
	case "read_file", "search_code":
		var args struct {
			Base bool `json:"base"`
		}
		if json.Unmarshal([]byte(tr.Arguments), &args) != nil {
			return false, false
		}
		return args.Base, !args.Base
	case "read_files":
		var args batchArgs
		if json.Unmarshal([]byte(tr.Arguments), &args) != nil || len(args.Files) == 0 {
			return false, false
		}
		base := args.Files[0].Base
		for _, file := range args.Files {
			if file.Base != base {
				return false, false
			}
		}
		return base, !base
	default:
		return false, false
	}
}
