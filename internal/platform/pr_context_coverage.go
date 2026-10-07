package platform

import "fmt"

// These are recording gaps, not semantic judgments. Even a fully populated
// structure is only static evidence; no path or runtime outcome is certified.
func findingPRCoverage(findings []Finding) []string {
	notes := []string{}
	for _, finding := range findings {
		prefix := fmt.Sprintf("Finding %s PR impact recording gap: ", finding.ID)
		p := finding.PRContext
		if p == nil && finding.Origin == deterministicFormattingOrigin {
			// Deterministic formatting-scope facts come from the pinned diff itself.
			continue
		}
		if p == nil && finding.AnchorType == "git_metadata" && finding.Metadata != nil {
			// Validated canonical metadata already contains both snapshot facts.
			continue
		}
		if p == nil {
			notes = append(notes, prefix+"no structured BASE/HEAD comparison or source-linked risk chain recorded")
			continue
		}
		if len(p.BeforeObservationIDs) == 0 {
			notes = append(notes, prefix+"BASE behavior lacks explicitly linked primary source observations")
		}
		if len(p.AfterObservationIDs) == 0 {
			notes = append(notes, prefix+"HEAD behavior lacks explicitly linked primary source observations")
		}
		if finding.AnchorType != "git_metadata" && len(p.Relationships) == 0 {
			notes = append(notes, prefix+"no source-linked relationships recorded; the risk path remains unestablished")
		}
		for _, edge := range p.Relationships {
			if edge.Certainty == "inferred" {
				notes = append(notes, prefix+"inferred relationships remain unverified even when unresolved_edges is empty")
				break
			}
		}
	}
	return notes
}
