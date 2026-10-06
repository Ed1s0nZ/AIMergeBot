package platform

import (
	"encoding/json"
	"fmt"
)

// Final coverage uses the same PR recording gaps as live navigation, including
// inspections which resolve without a finding. It never changes their claims.
func (t *auditTools) investigationPRCoverage(items []Investigation) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	metadataSources := t.primaryMetadataOnlySourcesLocked()
	notes := []string{}
	for _, item := range items {
		metadataOnly := metadataOnlyInvestigation(item, metadataSources)
		prefix := fmt.Sprintf("Investigation %s PR impact recording gap: ", item.ID)
		for _, gap := range prRecordingGaps(item.PRContext, metadataOnly) {
			switch gap {
			case "pr_context_missing":
				notes = append(notes, prefix+"no structured BASE/HEAD comparison or source-linked behavior/contract relationships recorded")
			case "base_sources_missing":
				notes = append(notes, prefix+"BASE behavior lacks explicitly linked primary source observations")
			case "head_sources_missing":
				notes = append(notes, prefix+"HEAD behavior lacks explicitly linked primary source observations")
			case "relationships_missing":
				notes = append(notes, prefix+"no source-linked relationships recorded; behavioral/contract connections remain unestablished")
			case "relationships_inferred":
				notes = append(notes, prefix+"inferred relationships remain unverified even when unresolved_edges is empty")
			}
		}
	}
	return notes
}

func metadataOnlyInvestigation(item Investigation, sources map[string]bool) bool {
	if len(item.ObservationIDs)+len(item.CounterObservationIDs) == 0 {
		return false
	}
	for _, ids := range [][]string{item.ObservationIDs, item.CounterObservationIDs} {
		for _, id := range ids {
			if !sources[id] {
				return false
			}
		}
	}
	return true
}

// Caller holds t.mu. Canonical, included metadata-only entries contain both
// Git snapshot facts without implying any execution path. No model field grants
// this exemption. Duplicate trace IDs conservatively invalidate the basis.
func (t *auditTools) primaryMetadataOnlySourcesLocked() map[string]bool {
	sources := map[string]bool{}
	seen := map[string]bool{}
	for _, tr := range t.trace {
		if tr.ObservationID == "" {
			continue
		}
		if seen[tr.ObservationID] {
			sources[tr.ObservationID] = false
			continue
		}
		seen[tr.ObservationID] = true
		sources[tr.ObservationID] = false
		if tr.Name != "get_change_metadata" || tr.Error != "" || (tr.Stage != "" && tr.Stage != "primary") {
			continue
		}
		var out toolOutput
		var args metadataArgs
		if json.Unmarshal([]byte(tr.Output), &out) != nil || json.Unmarshal([]byte(tr.Arguments), &args) != nil {
			continue
		}
		expected, ok := t.scope.Metadata[args.Path]
		if !ok || !expected.valid() || !expected.metadataOnly() {
			continue
		}
		if out.RepositoryID != 0 || !out.EvidenceEligible || out.Error != "" || out.ObservationID != tr.ObservationID || out.BaseSHA != t.snap.BaseSHA || out.HeadSHA != t.snap.HeadSHA || out.Metadata == nil {
			continue
		}
		canonical := expected.canonical()
		sources[tr.ObservationID] = out.Metadata.canonical() == canonical && out.Text == canonical
	}
	return sources
}
