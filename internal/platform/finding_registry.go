package platform

import (
	"context"
	"fmt"
	"sort"
)

// acceptFinding is the single entry point for both tool submissions and final proposals.
func (t *auditTools) acceptFinding(ctx context.Context, f Finding) (Finding, error) {
	if err := t.validateFindingLinks(f); err != nil {
		return Finding{}, err
	}
	result := AuditResult{Findings: []Finding{f}}
	if err := ValidateFindings(ctx, t.repo, t.snap, t.scope, &result); err != nil {
		return Finding{}, err
	}
	f = result.Findings[0]
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.findings == nil {
		t.findings = map[string]Finding{}
	}
	t.findings[f.ID] = f
	return f, nil
}
func (t *auditTools) acceptedFindings() []Finding {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := []string{}
	for id := range t.findings {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []Finding{}
	for _, id := range ids {
		out = append(out, t.findings[id])
	}
	return out
}
func (t *auditTools) mergeProposals(ctx context.Context, result *AuditResult) {
	for i, f := range result.Findings {
		if _, err := t.acceptFinding(ctx, f); err != nil {
			result.CoverageNotes = append(result.CoverageNotes, fmt.Sprintf("Rejected finding proposal %d: %s", i+1, err))
		}
	}
	result.Findings = t.acceptedFindings()
}
