package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool/utils"
)

func assertAssessmentSchema(t *testing.T, schema map[string]any) {
	t.Helper()
	props := schema["properties"].(map[string]any)
	if _, found := props["status"]; found {
		t.Fatal("legacy status advertised")
	}
	assessment := props["claim_assessment"].(map[string]any)
	want := []any{"evidence_supports_claim", "evidence_refutes_claim", "insufficient_evidence"}
	if !reflect.DeepEqual(assessment["enum"], want) {
		t.Fatalf("assessment enum: %v", assessment)
	}
	required, _ := schema["required"].([]any)
	found := false
	for _, field := range required {
		if field == "status" {
			t.Fatal("legacy status required")
		}
		found = found || field == "claim_assessment"
	}
	if !found {
		t.Fatal("assessment not required")
	}
	plan := props["plan"].(map[string]any)["items"].(map[string]any)
	question := plan["properties"].(map[string]any)["question"].(map[string]any)
	if !strings.Contains(question["description"].(string), "exact saved question") {
		t.Fatal("SDK omits immutable question guidance")
	}
	if _, ok := plan["properties"].(map[string]any)["status"]; !ok {
		t.Fatal("plan status erased")
	}
}

func TestInvestigationAssessmentSchema(t *testing.T) {
	info, err := utils.GoStruct2ToolInfo[investigationAssessmentUpdate]("update_investigation", "", utils.WithSchemaModifier(investigationAssessmentSchema))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(schema)
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	assertAssessmentSchema(t, data)
}

func TestInvestigationAssessmentCompatibilityAndTrace(t *testing.T) {
	for _, tc := range []struct {
		assessment, legacy, want string
		invalid                  bool
	}{
		{"evidence_supports_claim", "", "supported", false},
		{"evidence_refutes_claim", "", "rejected", false},
		{"insufficient_evidence", "", "investigating", false},
		{"evidence_supports_claim", "supported", "supported", false},
		{"", "supported", "supported", false},
		{"", "rejected", "rejected", false},
		{"", "investigating", "investigating", false},
		{"evidence_supports_claim", "rejected", "", true},
		{"evidence_refutes_claim", "supported", "", true},
		{"insufficient_evidence", "supported", "", true},
		{"safe", "", "", true},
		{"", "", "", true},
	} {
		t.Run(tc.assessment+"/"+tc.legacy, func(t *testing.T) {
			repo, snap, f, _ := sequenceFixture()
			tools := &auditTools{repo: repo, snap: snap, cache: map[string]string{}}
			source, _ := tools.file(context.Background(), readArgs{Path: f.File, Start: 1, End: 2})
			if source.Error != "" {
				t.Fatal(source.Error)
			}
			i := Investigation{ID: "assessment", Claim: "guard change is compatible", Plan: pendingPlan()}
			out, _ := tools.record(context.Background(), i)
			if out.Error != "" {
				t.Fatal(out.Error)
			}
			before, _ := json.Marshal(tools.investigations())
			i.Status = tc.legacy
			i.Evidence = []string{"source supports the actual claim"}
			i.Counterevidence = []string{"source refutes the actual claim"}
			i.ObservationIDs = []string{source.ObservationID}
			i.CounterObservationIDs = []string{source.ObservationID}
			for n := range i.Plan {
				i.Plan[n].Status = "checked"
				i.Plan[n].Reason = "fixture inspected, not semantic proof"
				i.Plan[n].ObservationIDs = []string{source.ObservationID}
			}
			out, _ = tools.updateAssessment(context.Background(), investigationAssessmentUpdate{i, tc.assessment})
			if (out.Error != "") != tc.invalid {
				t.Fatalf("output: %+v", out)
			}
			if tc.invalid {
				after, _ := json.Marshal(tools.investigations())
				if string(before) != string(after) {
					t.Fatal("rejected input mutated ledger")
				}
			} else if tools.investigations()[0].Status != tc.want {
				t.Fatal(tools.investigations())
			}
			raw, _ := json.Marshal(tools.trace[len(tools.trace)-1])
			if tc.assessment != "" && !strings.Contains(string(raw), tc.assessment) {
				t.Fatal("submitted assessment absent from trace")
			}
		})
	}
}

func TestInvestigationAssessmentRetainsEvidenceAndInputBudgetGate(t *testing.T) {
	repo, snap, _, _ := sequenceFixture()
	tools := &auditTools{repo: repo, snap: snap, cache: map[string]string{}}
	i := Investigation{ID: "assessment", Claim: "guard change is compatible"}
	tools.record(context.Background(), i)
	for _, assessment := range []string{"evidence_supports_claim", "evidence_refutes_claim"} {
		out, _ := tools.updateAssessment(context.Background(), investigationAssessmentUpdate{i, assessment})
		if out.Error == "" || tools.investigations()[0].Status != "investigating" {
			t.Fatal("source gate bypass")
		}
	}
	i.Claim = strings.Repeat("x", 8000)
	out, _ := tools.updateAssessment(context.Background(), investigationAssessmentUpdate{i, "insufficient_evidence"})
	if out.Error == "" || tools.investigations()[0].Claim != "guard change is compatible" {
		t.Fatal("input budget bypass")
	}
}
