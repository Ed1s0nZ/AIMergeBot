package platform

import "context"

// Common public completion boundaries preserve explicit review state even if
// execution stops before supplementary stages. Primary-only group fragments
// remain eligible for the single aggregate review instead of being finalized.
func (e *EinoAuditor) Audit(ctx context.Context, snap Snapshot, scope DiffScope) (AuditResult, []ToolTrace, error) {
	checkpointed := *e
	if !e.Config.PrimaryOnly {
		checkpointed.Config.Progress = claimReviewProgress(e.Config.Progress, snap, e.Config.VerifyFindings)
	}
	result, trace, err := checkpointed.audit(ctx, snap, scope)
	e.completeClaimReviewStates(snap, &result)
	return result, trace, err
}

func (e *EinoAuditor) AuditGroups(ctx context.Context, snap Snapshot, plan AuditPlan) (AuditResult, []ToolTrace, error) {
	checkpointed := *e
	checkpointed.Config.Progress = claimReviewProgress(e.Config.Progress, snap, e.Config.VerifyFindings)
	result, trace, err := checkpointed.auditGroups(ctx, snap, plan)
	e.completeClaimReviewStates(snap, &result)
	return result, trace, err
}

func (e *EinoAuditor) completeClaimReviewStates(snap Snapshot, result *AuditResult) {
	if e.Config.PrimaryOnly {
		return
	}
	completeClaimReviewStates(snap, result, e.Config.VerifyFindings)
}

func completeClaimReviewStates(snap Snapshot, result *AuditResult, enabled bool) {
	for _, i := range resolvedInvestigationOrder(result.Investigations) {
		item := &result.Investigations[i]
		if item.ClaimVerification != nil {
			continue
		}
		status, reason := "disabled", "系统设置已关闭独立复核。"
		if enabled {
			status, reason = "unavailable", "独立命题复核尚未完成，原调查保留。"
			result.CoverageNotes = append(result.CoverageNotes, "Independent investigation claim review unavailable: "+item.ID)
		}
		item.ClaimVerification = unavailableClaimVerification(snap, *item, "", status, reason)
	}
}

// Project explicit pending review state into durable checkpoints before a
// cancellation/revocation/recovery fence can freeze them. Never mutate the
// primary ledger or supplemental result: final review replaces this projection.
// Group checkpoints are projected at the aggregate callback, after IDs have
// been namespaced. Returned fragment ledgers remain eligible for aggregate review.
func claimReviewProgress(progress func(AuditResult, []ToolTrace) error, snap Snapshot, enabled bool) func(AuditResult, []ToolTrace) error {
	if progress == nil {
		return nil
	}
	return func(result AuditResult, trace []ToolTrace) error {
		result.Investigations = append([]Investigation(nil), result.Investigations...)
		result.CoverageNotes = append([]string(nil), result.CoverageNotes...)
		completeClaimReviewStates(snap, &result, enabled)
		return progress(result, trace)
	}
}
