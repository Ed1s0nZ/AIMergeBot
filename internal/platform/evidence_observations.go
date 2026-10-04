package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

func isSourceTool(name string) bool {
	switch name {
	case "read_file", "read_files", "search_code", "get_diff", "compare_files", "get_history", "git_blame", "search_history":
		return true
	}
	return false
}

// Called under t.mu. An observation link establishes source provenance, not call semantics.
func (t *auditTools) validateObservationIDs(ids []string) error {
	if len(ids) > 20 {
		return fmt.Errorf("at most 20 evidence observations allowed")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("duplicate observation reference")
		}
		seen[id] = true
		found := false
		for _, tr := range t.trace {
			if tr.ObservationID == id && tr.Error == "" && isSourceTool(tr.Name) {
				var out toolOutput
				if json.Unmarshal([]byte(tr.Output), &out) == nil && strings.TrimSpace(out.Text) != "" {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("observation %s is not a successful source observation", id)
		}
	}
	return nil
}
func (t *auditTools) validateFindingLinks(f Finding) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.validateObservationIDs(f.ObservationIDs); err != nil {
		return err
	}
	if f.InvestigationID != "" {
		if _, ok := t.ledger[f.InvestigationID]; !ok {
			return fmt.Errorf("unknown investigation")
		}
	}
	if f.Confidence != "supported" {
		return nil
	}
	item, ok := t.ledger[f.InvestigationID]
	if !ok || item.Status != "supported" || len(f.ObservationIDs) == 0 {
		return fmt.Errorf("supported finding requires a supported investigation and evidence observation links")
	}
	evidence := map[string]bool{}
	for _, id := range item.ObservationIDs {
		evidence[id] = true
	}
	matched := false
	for _, id := range f.ObservationIDs {
		if !evidence[id] {
			return fmt.Errorf("finding observation is not linked to investigation")
		}
		for _, tr := range t.trace {
			if tr.ObservationID == id {
				var out toolOutput
				_ = json.Unmarshal([]byte(tr.Output), &out)
				if strings.Contains(out.Text, f.Evidence) {
					matched = true
				}
			}
		}
	}
	if !matched {
		return fmt.Errorf("finding anchor snippet absent from referenced observations")
	}
	return nil
}
