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

type InvestigationRelationship struct {
	From           string   `json:"from" jsonschema:"minLength=1,maxLength=200" jsonschema_description:"Nonempty source endpoint. Use inspected facts or explicitly inferred endpoints; never invent a missing endpoint."`
	To             string   `json:"to" jsonschema:"minLength=1,maxLength=200" jsonschema_description:"Nonempty target endpoint. If unknown, omit this relationship and retain the gap in unresolved_edges; do not send an empty target."`
	Relation       string   `json:"relation" jsonschema:"minLength=1,maxLength=500" jsonschema_description:"Nonempty bounded description of the specific static connection, with actual uncertainty preserved."`
	Certainty      string   `json:"certainty" jsonschema:"enum=cited,enum=inferred" jsonschema_description:"cited requires inspected connection evidence; inferred preserves an unconfirmed relationship. Names alone do not establish a contract."`
	ObservationIDs []string `json:"observation_ids" jsonschema:"minItems=1,maxItems=8" jsonschema_description:"One to eight unique successful source observation IDs also linked at investigation level. Recording feedback is not source evidence."`
}

// Source links support review of these static claims, not proof of call semantics.
type PRInvestigationContext struct {
	BeforeObservationIDs []string                    `json:"before_observation_ids,omitempty"`
	AfterObservationIDs  []string                    `json:"after_observation_ids,omitempty"`
	Impact               []InvestigationFact         `json:"impact,omitempty"`
	Counterexamples      []InvestigationFact         `json:"counterexamples,omitempty"`
	Relationships        []InvestigationRelationship `json:"relationships,omitempty"`
	ChangeSummary        string                      `json:"change_summary"`
	Before               string                      `json:"before"`
	After                string                      `json:"after"`
	EntryPoints          []InvestigationFact         `json:"entry_points"`
	Guards               []InvestigationFact         `json:"guards"`
	UnresolvedEdges      []string                    `json:"unresolved_edges"`
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
	if !boundedFact(p.ChangeSummary, 800) || !boundedFact(p.Before, 800) || !boundedFact(p.After, 800) || len(p.EntryPoints) > 8 || len(p.Guards) > 8 || len(p.UnresolvedEdges) > 8 || len(p.Impact) > 8 || len(p.Counterexamples) > 8 || len(p.Relationships) > 8 {
		return fmt.Errorf("invalid PR context size or missing before/after/change summary")
	}
	allowed := map[string]bool{}
	for _, id := range append(append([]string{}, a.ObservationIDs...), a.CounterObservationIDs...) {
		allowed[id] = true
	}
	for _, facts := range [][]InvestigationFact{p.EntryPoints, p.Guards, p.Impact, p.Counterexamples} {
		for _, fact := range facts {
			if !boundedFact(fact.Statement, 500) || len(fact.ObservationIDs) < 1 || len(fact.ObservationIDs) > 8 {
				return fmt.Errorf("PR context facts need bounded statements and source observations")
			}
			seen := map[string]bool{}
			for _, id := range fact.ObservationIDs {
				if seen[id] {
					return fmt.Errorf("duplicate PR fact observation %q; include each source ID once", id)
				}
				if !allowed[id] {
					return fmt.Errorf("PR fact observation %q is absent from this investigation's observation_ids/counter_observation_ids; add the successful source ID to the submitted investigation list, or remove the unsupported fact reference", id)
				}
				seen[id] = true
			}
		}
	}

	validateIDs := func(ids []string, required bool) error {
		if len(ids) > 8 || (required && len(ids) == 0) {
			return fmt.Errorf("PR relationship needs 1–8 source observations")
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				return fmt.Errorf("duplicate PR relationship observation %q; include each source ID once", id)
			}
			if !allowed[id] {
				return fmt.Errorf("PR relationship observation %q is absent from this investigation's observation_ids/counter_observation_ids; add the successful source ID to the submitted investigation list, or remove the unsupported relationship reference", id)
			}
			seen[id] = true
		}
		return nil
	}
	for _, ids := range [][]string{p.BeforeObservationIDs, p.AfterObservationIDs} {
		if err := validateIDs(ids, false); err != nil {
			return err
		}
	}
	for i, edge := range p.Relationships {
		for _, field := range []struct {
			name, value string
			limit       int
		}{{"from", edge.From, 200}, {"to", edge.To, 200}, {"relation", edge.Relation, 500}} {
			if !boundedFact(field.value, field.limit) {
				return fmt.Errorf("pr_context.relationships[%d].%s must be nonempty valid UTF-8 without NUL and at most %d characters; explicitly supply the endpoint or relation. If unknown, omit this relationship and preserve the gap in pr_context.unresolved_edges; never invent a value", i, field.name, field.limit)
			}
		}
		if edge.Certainty != "cited" && edge.Certainty != "inferred" {
			return fmt.Errorf("pr_context.relationships[%d].certainty must be cited or inferred; cited requires inspected connection evidence and inferred preserves uncertainty. If the relationship is unknown, omit it and preserve the gap in pr_context.unresolved_edges", i)
		}
		if err := validateIDs(edge.ObservationIDs, true); err != nil {
			return err
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
