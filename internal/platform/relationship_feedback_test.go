package platform

import (
	"strings"
	"testing"
)

func TestRelationshipFieldFeedbackIsBoundedAndKeepsValidation(t *testing.T) {
	valid := InvestigationRelationship{From: "source", To: "target", Relation: "conditional connection", Certainty: "inferred", ObservationIDs: []string{"source-id"}}
	for _, tc := range []struct {
		name, field string
		mutate      func(*InvestigationRelationship)
	}{
		{"empty_from", "from", func(e *InvestigationRelationship) { e.From = "" }},
		{"blank_to", "to", func(e *InvestigationRelationship) { e.To = "  \n" }},
		{"empty_to", "to", func(e *InvestigationRelationship) { e.To = "" }},
		{"large_from", "from", func(e *InvestigationRelationship) { e.From = strings.Repeat("PRIVATE_VALUE", 20) }},
		{"large_to", "to", func(e *InvestigationRelationship) { e.To = strings.Repeat("字", 201) }},
		{"large_relation", "relation", func(e *InvestigationRelationship) { e.Relation = strings.Repeat("字", 501) }},
		{"nul", "to", func(e *InvestigationRelationship) { e.To = "PRIVATE_VALUE\x00" }},
		{"invalid_utf8", "relation", func(e *InvestigationRelationship) { e.Relation = string([]byte{0xff}) }},
		{"certainty", "certainty", func(e *InvestigationRelationship) { e.Certainty = "PRIVATE_VALUE" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPRContext("source-id")
			edge := valid
			tc.mutate(&edge)
			p.Relationships = []InvestigationRelationship{valid, edge}
			err := validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source-id"}})
			if err == nil || !strings.Contains(err.Error(), "pr_context.relationships[1]."+tc.field) || strings.Contains(err.Error(), "PRIVATE_VALUE") || len(err.Error()) > 500 || !strings.Contains(err.Error(), "unresolved_edges") {
				t.Fatal("missing safe actionable field feedback", err)
			}
		})
	}
	for _, certainty := range []string{"cited", "inferred"} {
		p := validPRContext("source-id")
		edge := valid
		edge.From = strings.Repeat("字", 200)
		edge.To = strings.Repeat("🙂", 200)
		edge.Relation = strings.Repeat("字", 500)
		edge.Certainty = certainty
		p.Relationships = []InvestigationRelationship{edge}
		if err := validatePRContext(Investigation{PRContext: p, ObservationIDs: []string{"source-id"}}); err != nil {
			t.Fatal("original Unicode limits changed", err)
		}
	}
}
