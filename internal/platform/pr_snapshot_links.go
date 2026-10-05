package platform

import (
	"encoding/json"
	"fmt"
)

// Called under t.mu, after successful-source and investigation ownership checks.
func (t *auditTools) validatePRSnapshotLinks(p *PRInvestigationContext) error {
	if p == nil {
		return nil
	}
	for _, side := range []struct {
		base bool
		ids  []string
	}{{true, p.BeforeObservationIDs}, {false, p.AfterObservationIDs}} {
		for _, id := range side.ids {
			found := false
			for _, tr := range t.trace {
				if tr.ObservationID != id || tr.Error != "" {
					continue
				}
				var out toolOutput
				if json.Unmarshal([]byte(tr.Output), &out) != nil || out.RepositoryID != 0 {
					continue
				}
				switch tr.Name {
				case "get_diff", "compare_files", "get_change_metadata":
					found = true
				case "read_file", "search_code":
					var a struct {
						Base bool `json:"base"`
					}
					if json.Unmarshal([]byte(tr.Arguments), &a) == nil && a.Base == side.base {
						found = true
					}
				case "read_files":
					var a batchArgs
					if json.Unmarshal([]byte(tr.Arguments), &a) == nil && len(a.Files) > 0 {
						found = true
						for _, f := range a.Files {
							if f.Base != side.base {
								found = false
								break
							}
						}
					}
				}
			}
			if !found {
				return fmt.Errorf("PR before/after observation %s does not read the required primary snapshot side", id)
			}
		}
	}
	return nil
}
