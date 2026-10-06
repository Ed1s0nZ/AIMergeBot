package platform

// Recording feedback contains bounded codes, never model text or source facts.
// A populated record is still a static claim, not proof of semantic correctness.
func prRecordingGaps(p *PRInvestigationContext, metadata bool) []string {
	if p == nil {
		if metadata {
			return nil
		}
		return []string{"pr_context_missing"}
	}
	gaps := []string{}
	if len(p.BeforeObservationIDs) == 0 {
		gaps = append(gaps, "base_sources_missing")
	}
	if len(p.AfterObservationIDs) == 0 {
		gaps = append(gaps, "head_sources_missing")
	}
	if !metadata && len(p.Relationships) == 0 {
		gaps = append(gaps, "relationships_missing")
	}
	for _, edge := range p.Relationships {
		if edge.Certainty == "inferred" {
			gaps = append(gaps, "relationships_inferred")
			break
		}
	}
	return gaps
}
func investigationRecordingGaps(a Investigation) []string {
	gaps := []string{}
	if len(a.Plan) == 0 {
		gaps = append(gaps, "plan_missing")
	} else {
		kinds := map[string]bool{}
		unfinished := false
		for _, task := range a.Plan {
			kinds[task.Kind] = true
			unfinished = unfinished || task.Status != "checked"
		}
		for _, kind := range verificationKinds {
			if !kinds[kind] {
				gaps = append(gaps, "plan_missing_core_aspects")
				break
			}
		}
		if unfinished {
			gaps = append(gaps, "plan_unfinished")
		}
	}
	return append(gaps, prRecordingGaps(a.PRContext, false)...)
}

const recordingFeedbackGuidance = " Investigation and submit tools may return server-owned recording_gaps (navigation only, never source evidence): plan_missing/plan_missing_core_aspects/plan_unfinished identify unfinished recording; pr_context_missing/base_sources_missing/head_sources_missing/relationships_missing identify absent PR source links; relationships_inferred means uncertainty remains. For base_sources_missing set pr_context.before_observation_ids to actual primary BASE-read IDs; for head_sources_missing set pr_context.after_observation_ids to actual primary HEAD-read IDs. Include those IDs in the investigation top-level observation_ids or counter_observation_ids too. For relationships_missing populate pr_context.relationships only when inspected connection evidence supports from/to/relation; otherwise preserve unresolved_edges and coverage_notes. Correct records only with actual successful source observations. Explain genuinely unknown relations in unresolved_edges/coverage_notes; never invent a cited edge to clear a reminder. When updating an investigation after submitting a finding, resubmit the finding if its saved PR context must change; accepted findings retain their submission snapshot. Missing context or recording alone is not a vulnerability."
