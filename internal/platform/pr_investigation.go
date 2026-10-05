package platform

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type InvestigationFact struct {
	Statement      string   `json:"statement"`
	ObservationIDs []string `json:"observation_ids"`
}

// Source links support review of these static claims, not proof of call semantics.
type PRInvestigationContext struct {
	ChangeSummary   string              `json:"change_summary"`
	Before          string              `json:"before"`
	After           string              `json:"after"`
	EntryPoints     []InvestigationFact `json:"entry_points"`
	Guards          []InvestigationFact `json:"guards"`
	UnresolvedEdges []string            `json:"unresolved_edges"`
}

func boundedFact(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, '\x00')
}

// Caller holds t.mu; all observation IDs have already passed source validation.
func validatePRContext(a Investigation) error {
	p := a.PRContext
	if p == nil {
		return nil
	}
	if !boundedFact(p.ChangeSummary, 800) || !boundedFact(p.Before, 800) || !boundedFact(p.After, 800) || len(p.EntryPoints) > 8 || len(p.Guards) > 8 || len(p.UnresolvedEdges) > 8 {
		return fmt.Errorf("invalid PR context size or missing before/after/change summary")
	}
	allowed := map[string]bool{}
	for _, id := range append(append([]string{}, a.ObservationIDs...), a.CounterObservationIDs...) {
		allowed[id] = true
	}
	for _, facts := range [][]InvestigationFact{p.EntryPoints, p.Guards} {
		for _, fact := range facts {
			if !boundedFact(fact.Statement, 500) || len(fact.ObservationIDs) < 1 || len(fact.ObservationIDs) > 8 {
				return fmt.Errorf("PR context facts need bounded statements and source observations")
			}
			seen := map[string]bool{}
			for _, id := range fact.ObservationIDs {
				if !allowed[id] || seen[id] {
					return fmt.Errorf("PR context fact observations must be unique and linked to this investigation")
				}
				seen[id] = true
			}
		}
	}
	for _, edge := range p.UnresolvedEdges {
		if !boundedFact(edge, 500) {
			return fmt.Errorf("invalid unresolved PR relation")
		}
	}
	return nil
}

func clonePRContext(p *PRInvestigationContext) *PRInvestigationContext {
	if p == nil {
		return nil
	}
	var copy PRInvestigationContext
	raw, _ := json.Marshal(p)
	_ = json.Unmarshal(raw, &copy)
	return &copy
}
