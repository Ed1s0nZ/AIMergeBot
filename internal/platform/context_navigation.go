package platform

import (
	"context"
	"encoding/json"
	"fmt"
)

func (t *auditTools) contextNavigation(ctx context.Context) (string, error) {
	if len(contextPolicyItems(t.snap)) == 0 {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	out, err := t.contextRepositories(ctx, struct{}{})
	if err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("fixed context preflight failed: %s", out.Error)
	}
	t.mu.Lock()
	progressError := t.progressError
	t.mu.Unlock()
	if progressError != "" {
		return "", fmt.Errorf("fixed context preflight checkpoint failed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return "\nConfigured fixed context navigation (not source evidence; available sources can be investigated with repository-scoped tools; uninspected is not unavailable):\n" + string(raw), nil
}

// A source read proves inspection only. It does not establish relevance or a
// call relation, and later independent reads do not retroactively cover primary.
func primaryContextCoverage(snap Snapshot, trace []ToolTrace) []string {
	inspected := map[int]bool{}
	for _, tr := range trace {
		if tr.Stage != "" && tr.Stage != "primary" || tr.Error != "" || !isSourceTool(tr.Name) {
			continue
		}
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) == nil && out.RepositoryID != 0 && out.EvidenceEligible && out.Error == "" && out.Text != "" && observationAtSnapshot(out, snap) {
			inspected[out.RepositoryID] = true
		}
	}
	notes := []string{}
	for _, source := range contextPolicyItems(snap) {
		if !inspected[source.ProjectID] {
			notes = append(notes, fmt.Sprintf("Primary investigation did not inspect configured fixed context repository %d at %s; relevance and downstream protections remain undetermined", source.ProjectID, source.SHA))
		}
	}
	return notes
}
