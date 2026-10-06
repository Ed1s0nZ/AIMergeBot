package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRecordPRContextThroughSDKPreservesOmittedContextAndSchema(t *testing.T) {
	repo, snap, _, _ := sequenceFixture()
	var calls atomic.Int32
	var storedContext *PRInvestigationContext
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var request struct {
			Tools []struct {
				Function struct {
					Name       string
					Parameters struct{ Properties map[string]json.RawMessage }
				}
			}
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if n == 1 {
			schemas := 0
			for _, tool := range request.Tools {
				switch tool.Function.Name {
				case "record_hypothesis", "update_investigation", "record_pr_context", "submit_finding":
					assertRelationshipSDKSchema(t, tool.Function.Parameters.Properties["pr_context"])
					schemas++
				}
			}
			if schemas != 4 {
				t.Error("shared relationship schema missing from real SDK tools", schemas)
			}
			found := false
			for _, tool := range request.Tools {
				if tool.Function.Name == "record_pr_context" {
					found = true
					for _, field := range []string{"id", "claim", "pr_context"} {
						if _, ok := tool.Function.Parameters.Properties[field]; !ok {
							t.Error("missing context schema", field)
						}
					}
					for _, field := range []string{"plan", "status", "evidence", "claim_assessment", "claim_verification"} {
						if _, ok := tool.Function.Parameters.Properties[field]; ok {
							t.Error("unrelated mutable schema field", field)
						}
					}
				}
			}
			if !found {
				t.Error("missing typed context tool")
			}
		}
		inv := Investigation{ID: "inspection", Claim: "Inspected operation", ObservationIDs: []string{"observation-2", "observation-3", "observation-4"}, Evidence: []string{"Inspected source"}, NextSteps: []string{"Keep static limits"}, Plan: pendingPlan()}
		name, args := "read_file", `{"path":"service.any","base":true,"start":1,"end":3}`
		switch n {
		case 2:
			args = `{"path":"service.any","base":false,"start":1,"end":3}`
		case 3:
			args = `{"path":"caller.any","start":1,"end":3}`
		case 4:
			name = "record_hypothesis"
			b, _ := json.Marshal(inv)
			args = string(b)
		case 5:
			name = "record_pr_context"
			storedContext = &PRInvestigationContext{ChangeSummary: "Inspected operation", Before: "BASE inspected", After: "HEAD inspected", BeforeObservationIDs: []string{"observation-2"}, AfterObservationIDs: []string{"observation-3"}, Relationships: []InvestigationRelationship{{From: "caller.any", To: "service.any", Relation: "caller dispatches service(request)", Certainty: "cited", ObservationIDs: []string{"observation-4", "observation-3"}}}, UnresolvedEdges: []string{"Runtime not executed"}}
			b, _ := json.Marshal(prContextRecording{ID: inv.ID, Claim: inv.Claim, PRContext: storedContext})
			args = string(b)
		case 6:
			name = "update_investigation"
			for i := range inv.Plan {
				inv.Plan[i].Status = "checked"
				inv.Plan[i].Reason = "Source inspected"
				inv.Plan[i].ObservationIDs = []string{"observation-3"}
			}
			b, _ := json.Marshal(investigationAssessmentUpdate{Investigation: inv, ClaimAssessment: "evidence_supports_claim"})
			args = string(b)
		case 7:
			for _, m := range request.Messages {
				if m.Role == "system" && !strings.Contains(m.Content, `"next_recording_action":"summarize_with_limits"`) {
					t.Error("stored context not reflected in primary progress", m.Content)
				}
			}
		}
		message := map[string]any{"role": "assistant"}
		finish := "tool_calls"
		if n < 7 {
			message["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}
		} else {
			message["content"] = `{"findings":[],"summary":"static fixture only","coverage_notes":["Runtime not executed"]}`
			finish = "stop"
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 7, MaxToolCalls: 8}}
	result, trace, err := auditor.Audit(context.Background(), snap, DiffScope{})
	if err != nil || calls.Load() != 7 || len(result.Investigations) != 1 {
		t.Fatal(err, calls.Load(), result)
	}
	inv := result.Investigations[0]
	if inv.Status != "supported" || inv.PRContext == nil || len(inv.PRContext.Relationships) != 1 || inv.PRContext.Before != storedContext.Before || len(inv.Plan) != 4 {
		t.Fatal(inv)
	}
	for _, note := range result.CoverageNotes {
		if strings.Contains(note, "PR impact recording gap") {
			t.Fatal("context not saved", result)
		}
	}
	contextCalls := 0
	for _, tr := range trace {
		if tr.Name == "record_pr_context" {
			contextCalls++
			if tr.Error != "" {
				t.Fatal(tr)
			}
		}
	}
	if contextCalls != 1 || inv.ClaimVerification == nil || inv.ClaimVerification.Status != "disabled" {
		t.Fatal(contextCalls, inv)
	}
}

func assertRelationshipSDKSchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var context struct{ Properties map[string]json.RawMessage }
	if json.Unmarshal(raw, &context) != nil {
		t.Error("invalid SDK PR context schema")
		return
	}
	var edges struct {
		Items struct {
			Required   []string
			Properties map[string]struct {
				Type      string
				MinLength int
				MaxLength int
				MinItems  int
				MaxItems  int
				Enum      []string
			}
		}
	}
	if json.Unmarshal(context.Properties["relationships"], &edges) != nil {
		t.Error("invalid SDK relationship schema")
		return
	}
	for _, field := range []string{"from", "to", "relation", "certainty", "observation_ids"} {
		required := false
		for _, name := range edges.Items.Required {
			required = required || name == field
		}
		if !required {
			t.Error("relationship field not required", field)
		}
	}
	for _, name := range []string{"from", "to", "relation"} {
		field := edges.Items.Properties[name]
		limit := 200
		if name == "relation" {
			limit = 500
		}
		if field.Type != "string" || field.MinLength != 1 || field.MaxLength != limit {
			t.Error("SDK bounds differ from server", name, field)
		}
	}
	field := edges.Items.Properties["certainty"]
	if len(field.Enum) != 2 || field.Enum[0] != "cited" || field.Enum[1] != "inferred" {
		t.Error("wrong certainty enum", field.Enum)
	}
	ids := edges.Items.Properties["observation_ids"]
	if ids.Type != "array" || ids.MinItems != 1 || ids.MaxItems != 8 {
		t.Error("wrong SDK source ID bounds", ids)
	}
}
