package platform

import (
	"encoding/json"
	"slices"
)

// A next action is advice based on runtime metadata, never a semantic verdict.
func nextRecordingAction(state primaryRecordingProgress) string {
	if state.LedgerCount == 0 {
		if len(state.BaseSourceIDs)+len(state.HeadSourceIDs) == 0 {
			return "inspect_changed_source"
		}
		return "record_changed_behavior"
	}
	for _, step := range []struct {
		gaps   []string
		action string
	}{
		{[]string{"plan_missing", "plan_missing_core_aspects"}, "repair_plan"},
		{[]string{"plan_unfinished"}, "inspect_plan"},
		{[]string{"pr_context_missing"}, "link_pr_context"},
		{[]string{"base_sources_missing", "head_sources_missing"}, "link_pr_sides"},
	} {
		for _, gap := range step.gaps {
			if slices.Contains(state.RecordingGaps, gap) {
				return step.action
			}
		}
	}
	if len(state.UninspectedContextIDs) > 0 {
		return "inspect_context"
	}
	if slices.Contains(state.RecordingGaps, "relationships_missing") {
		return "record_relationships"
	}
	if state.UnresolvedLedgerCount > 0 {
		return "resolve_hypotheses"
	}
	if len(state.EligibleRecordingCorrections) > 0 {
		return "resolve_recording_errors"
	}
	return "summarize_with_limits"
}

func recordingActionGuidance(action string) string {
	switch action {
	case "inspect_changed_source":
		return "Read the changed behavior at the fixed BASE/HEAD first. The diff and filename inventory do not replace successful source-tool observations. If a required read is unavailable, preserve its actual limitation."
	case "record_changed_behavior":
		return "Next call record_hypothesis for the inspected changed behavior, even if it appears to be a security improvement or has no finding. State the actual claim neutrally and link successful source IDs. Include four concise pending tasks with kinds input_control, pr_causality, guards, outcome; add contract when relevant. Do not skip the inspection record and return a no-findings summary. Pending tasks need questions, not invented checked evidence."
	case "repair_plan":
		return "Use update_investigation to add missing plan aspects input_control, pr_causality, guards, outcome as pending tasks. Preserve existing task IDs/kinds/questions. Do not mark a task checked without its actual source IDs and factual reason."
	case "inspect_plan":
		return "Inspect the source needed by pending/unavailable plan tasks, then update_investigation with the actual factual reasons and linked source IDs. Keep necessary missing evidence unresolved; a checked task records inspection, not exploitability."
	case "link_pr_context":
		return "Use update_investigation to record concise pr_context: change_summary, before, after, source-linked entry_points and guards. Link actual BASE/HEAD and caller/contract observations; preserve unresolved_edges. Never convert filename enumeration into source facts."
	case "link_pr_sides":
		return "Read the missing primary BASE/HEAD behavior and set pr_context.before_observation_ids/after_observation_ids, also linked in the investigation's top-level source IDs. Preserve existing plan and context; missing snapshots remain limitations."
	case "inspect_context":
		return "Investigate relevant administrator-authorized fixed context contracts using scoped tools. An uninspected repository is not unavailable. Record which connection evidence exists and which necessary relation remains unknown; do not equate matching names with a contract."
	case "record_relationships":
		return "Use update_investigation to record pr_context.relationships only for connections supported by inspected caller/transform/contract source observations. Cite both endpoints and connection evidence where needed. If the connection is unknown, preserve unresolved_edges and coverage limitations; do not invent an edge to clear this reminder."
	case "resolve_hypotheses":
		return "Explicitly update each inspected hypothesis using claim_assessment: evidence_supports_claim if evidence supports that actual claim; evidence_refutes_claim only if counterevidence refutes it; insufficient_evidence for necessary missing evidence. A compatibility or guard-improvement claim may be supported without a finding."
	case "resolve_recording_errors":
		return "Explicitly pass relevant eligible_recording_corrections pairs to resolve_recording_errors. These same-statement candidates have not retired errors; absent candidates do not imply all errors are solved. Preserve history and actual source/coverage gaps."
	default:
		return "Summarize only the inspected behavior and actual limitations. No remaining listed recording action proves safety, semantic completeness, or runtime reproduction. Submit genuinely evidenced PR findings when appropriate; empty findings is allowed."
	}
}

func recordingProgressGuidance(state primaryRecordingProgress) string {
	raw, _ := json.Marshal(state)
	guidance := "\nServer-owned recording progress (navigation only, not source evidence): " + string(raw) + ". Next recording action: " + recordingActionGuidance(state.NextRecordingAction)
	guidance += " This advice stays within the original tool/decision budget; the final-decision strict JSON instruction takes precedence. Recent source IDs are candidates for deliberate linkage, not proof of relevance or connection. Preserve actual unknowns. record_hypothesis always creates investigating status. A supported compatibility/no-risk claim needs no finding. Neither status nor checked tasks proves a vulnerability or runtime reproduction."
	if len(state.UninspectedContextIDs) > 0 {
		guidance += " Uninspected configured context is not unavailable: use repository-scoped tools to check relevant contracts before judging compatibility."
	}
	if len(state.EligibleRecordingCorrections) > 0 && state.NextRecordingAction != "resolve_recording_errors" {
		guidance += " eligible_recording_corrections lists at most four validated same-statement recording pairs; explicitly resolve relevant pairs before final JSON. Do not change statements to obtain a pair or treat it as source evidence."
	}
	return guidance
}
