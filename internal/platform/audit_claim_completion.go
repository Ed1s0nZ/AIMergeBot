package platform

import "context"

// Common public completion boundaries preserve explicit review state even if
// execution stops before supplementary stages. Primary-only group fragments
// remain eligible for the single aggregate review instead of being finalized.
func (e *EinoAuditor) Audit(ctx context.Context, snap Snapshot, scope DiffScope) (AuditResult, []ToolTrace, error) {
	result, trace, err := e.audit(ctx, snap, scope)
	e.completeClaimReviewStates(snap, &result)
	return result, trace, err
}

func (e *EinoAuditor) AuditGroups(ctx context.Context, snap Snapshot, plan AuditPlan) (AuditResult, []ToolTrace, error) {
	result, trace, err := e.auditGroups(ctx, snap, plan)
	e.completeClaimReviewStates(snap, &result)
	return result, trace, err
}

func (e *EinoAuditor) completeClaimReviewStates(snap Snapshot, result *AuditResult) {
	if e.Config.PrimaryOnly {
		return
	}
	for _, i := range resolvedInvestigationOrder(result.Investigations) {
		item := &result.Investigations[i]
		if item.ClaimVerification != nil {
			continue
		}
		status, reason := "disabled", "系统设置已关闭独立复核。"
		if e.Config.VerifyFindings {
			status, reason = "unavailable", "审计未进入或未完成独立命题复核，原调查保留。"
			result.CoverageNotes = append(result.CoverageNotes, "Independent investigation claim review unavailable: "+item.ID)
		}
		item.ClaimVerification = unavailableClaimVerification(snap, *item, "", status, reason)
	}
}
