package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/eino-contrib/jsonschema"
)

// Legacy status remains decodable, but is not advertised to model callers.
// The persisted investigation contract remains unchanged.
type investigationAssessmentUpdate struct {
	Investigation
	ClaimAssessment string `json:"claim_assessment,omitempty"`
}

func investigationAssessmentSchema(name string, typ reflect.Type, _ reflect.StructTag, s *jsonschema.Schema) {
	if name == "claim_assessment" {
		s.Enum = []any{"evidence_supports_claim", "evidence_refutes_claim", "insufficient_evidence"}
		s.Description = "Whether inspected evidence supports or refutes the exact claim, independent of whether a vulnerability was found."
	}
	if name != "_root" || typ != reflect.TypeOf(investigationAssessmentUpdate{}) {
		return
	}
	s.Properties.Delete("status")
	required := make([]string, 0, len(s.Required)+1)
	for _, field := range s.Required {
		if field != "status" && field != "claim_assessment" {
			required = append(required, field)
		}
	}
	s.Required = append(required, "claim_assessment")
}

func normalizeInvestigationAssessment(a investigationAssessmentUpdate) (Investigation, error) {
	i := a.Investigation
	if a.ClaimAssessment == "" {
		// The original ledger gate validates legacy status and evidence.
		return i, nil
	}
	status := ""
	switch a.ClaimAssessment {
	case "evidence_supports_claim":
		status = "supported"
	case "evidence_refutes_claim":
		status = "rejected"
	case "insufficient_evidence":
		status = "investigating"
	default:
		return Investigation{}, fmt.Errorf("invalid claim_assessment; use evidence_supports_claim, evidence_refutes_claim or insufficient_evidence")
	}
	if i.Status != "" && i.Status != status {
		return Investigation{}, fmt.Errorf("claim_assessment conflicts with legacy status; assess the exact claim consistently")
	}
	i.Status = status
	return i, nil
}

func (t *auditTools) updateAssessment(_ context.Context, a investigationAssessmentUpdate) (toolOutput, error) {
	raw, _ := json.Marshal(a)
	i, err := normalizeInvestigationAssessment(a)
	if len(raw) > 8000 {
		err = fmt.Errorf("investigation input exceeds 8000 UTF-8 bytes; shorten repeated evidence and retain concise source-linked facts")
	}
	if err != nil {
		return t.invoke("update_investigation", a, func() (toolOutput, error) { return toolOutput{}, err })
	}
	return t.ledgerChangeWithInput("update_investigation", i, a)
}
