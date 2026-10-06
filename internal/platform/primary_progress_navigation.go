package platform

import "encoding/json"

type primaryRecordingProgress struct {
	PrimaryHeadInventoryState    string                `json:"primary_head_inventory_state"`
	PrimaryHeadListedFileCount   int                   `json:"primary_head_listed_file_count"`
	EligibleRecordingCorrections []recordingCorrection `json:"eligible_recording_corrections,omitempty"`
	UninspectedContextIDs        []int                 `json:"uninspected_context_ids,omitempty"`
	LedgerCount                  int                   `json:"ledger_count"`
	UnresolvedLedgerCount        int                   `json:"unresolved_ledger_count"`
	RecordingGaps                []string              `json:"recording_gaps,omitempty"`
	BaseSourceIDs                []string              `json:"recent_primary_base_source_ids,omitempty"`
	HeadSourceIDs                []string              `json:"recent_primary_head_source_ids,omitempty"`
}

// This projects runtime metadata only. Model claims, reasons, paths and source
// snippets must never be interpolated into the trusted system instruction.
func (t *auditTools) primaryProgressNavigation() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	state := primaryRecordingProgress{LedgerCount: len(t.ledger), EligibleRecordingCorrections: t.eligibleRecordingCorrectionsLocked()}
	state.PrimaryHeadInventoryState, state.PrimaryHeadListedFileCount = t.primaryHeadInventoryLocked()
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
		if item.Status != "supported" && item.Status != "rejected" {
			state.UnresolvedLedgerCount++
		}
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
	return "\nServer-owned recording progress (navigation only, not source evidence): " + string(raw) + ". Uninspected configured context is not unavailable: use repository-scoped tools to check relevant contracts before judging compatibility. If ledger_count is zero, record a source-linked changed-behavior inspection with a plan, which may conclude rejected/no-risk; do not invent a finding. Recent source IDs are candidates for deliberate linkage, not proof of relevance or connection. Preserve actual unknowns. eligible_recording_corrections lists at most four validated same-statement recording pairs; explicitly pass relevant pairs to resolve_recording_errors before final JSON. Listing a pair has not retired its pending failure or certified its claim. Do not change statements to obtain a pair; absent pairs do not mean all failures are solved. record_hypothesis always creates investigating status, even when its input plan is already checked. Before final JSON, explicitly update each inspected hypothesis using its saved id and successful source IDs: supported means the recorded claim is supported, rejected means counterevidence refutes that claim; neither status alone warrants a finding. A supported compatibility/no-risk claim needs no finding. Keep investigating when necessary evidence is missing; checked tasks alone never resolve a hypothesis. Do not change a claim or hide unknown relations merely to clear unresolved_ledger_count."
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
