package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	eo "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

func TestRelationshipFieldFeedbackSDKRequiresExplicitCorrectionAndKeepsHistory(t *testing.T) {
	tools, a, _ := contextRecordingFixture(t)
	before := tools.ledger[a.ID]
	wrong := a
	wrong.PRContext = clonePRContext(a.PRContext)
	wrong.PRContext.Relationships[0].To = ""
	var requests atomic.Int32
	failedID, correctedID := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		n := requests.Add(1)
		for _, m := range req.Messages {
			if m.Role == "tool" {
				var out toolOutput
				if json.Unmarshal([]byte(m.Content), &out) != nil {
					t.Error("bad SDK tool output")
					continue
				}
				if n == 2 {
					failedID = out.ObservationID
					if !strings.Contains(out.Error, "pr_context.relationships[0].to") || !strings.Contains(out.Error, "unresolved_edges") || out.EvidenceEligible {
						t.Error("missing actionable non-source failure", out)
					}
					if !reflect.DeepEqual(tools.ledger[a.ID], before) {
						t.Error("failed recording changed ledger")
					}
				}
				if n == 3 && out.Error == "" {
					correctedID = out.ObservationID
				}
			}
		}
		name := "record_pr_context"
		var input any = wrong
		switch n {
		case 2:
			input = a
		case 3:
			name = "resolve_recording_errors"
			input = recordingCorrectionsArgs{Corrections: []recordingCorrection{{FailedObservationID: failedID, CorrectedObservationID: correctedID}}}
		}
		msg := map[string]any{"role": "assistant"}
		finish := "tool_calls"
		if n <= 3 {
			raw, _ := json.Marshal(input)
			msg["tool_calls"] = []any{map[string]any{"id": "fixture", "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}}}
		} else {
			msg["content"] = "recording fixture complete, no semantic accuracy claim"
			finish = "stop"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}})
	}))
	defer server.Close()
	model, err := eo.NewChatModel(context.Background(), &eo.ChatModelConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "field-feedback-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := tools.register()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{ToolCallingModel: model, ToolsConfig: compose.ToolsNodeConfig{Tools: registered}, MaxStep: 7})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agent.Generate(context.Background(), []*schema.Message{schema.UserMessage("Exercise explicit recording correction on a fixed local fixture")})
	if err != nil || requests.Load() != 4 || tools.calls != 6 { // Three initial sources plus three actual recording calls.
		t.Fatal(err, requests.Load(), tools.calls)
	}
	got := tools.ledger[a.ID]
	if got.PRContext == nil || got.PRContext.Relationships[0].To != a.PRContext.Relationships[0].To || got.PRContext.Relationships[0].Certainty != "inferred" || got.Claim != before.Claim || got.Status != before.Status || !reflect.DeepEqual(got.Plan, before.Plan) {
		t.Fatal("explicit correction altered unrelated facts", got)
	}
	if len(tools.trace) != 6 || tools.trace[3].ObservationID != failedID || tools.trace[3].Error == "" || tools.trace[4].ObservationID != correctedID || tools.trace[4].Error != "" || tools.trace[5].Name != "resolve_recording_errors" || tools.trace[5].Error != "" {
		t.Fatal("original failed trace or explicit recovery lost", tools.trace)
	}
}
