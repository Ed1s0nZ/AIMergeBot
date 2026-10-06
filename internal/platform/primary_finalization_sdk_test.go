package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/eino/compose"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrimaryRecordingFinalizationThroughSDKUsesOriginalBudgetAndUsage(t *testing.T) {
	repo, snap, _, _ := sequenceFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		system := ""
		for _, m := range req.Messages {
			if m.Role == "system" {
				system += m.Content
			}
		}
		if !strings.Contains(system, fmt.Sprintf("decision %d of 9", n)) || strings.Count(system, "Server-owned decision budget") != 1 {
			t.Error("actual decision count not propagated", n, system)
		}
		inv := Investigation{ID: "inspection", Claim: "Inspected source", ObservationIDs: []string{"observation-2", "observation-3"}, Evidence: []string{"Source inspected"}, Plan: pendingPlan()}
		for i := range inv.Plan {
			inv.Plan[i].Status = "checked"
			inv.Plan[i].Reason = "Source inspected"
			inv.Plan[i].ObservationIDs = []string{"observation-3"}
		}
		name, args := "read_file", `{"path":"service.any","base":true,"start":1,"end":3}`
		switch n {
		case 2:
			args = `{"path":"service.any","start":1,"end":3}`
		case 3:
			name = "record_hypothesis"
			args = mustRecordingJSON(t, inv)
		case 4:
			name = "update_investigation"
			args = mustRecordingJSON(t, investigationAssessmentUpdate{Investigation: inv, ClaimAssessment: "evidence_supports_claim"})
		case 6:
			if !strings.Contains(req.Messages[len(req.Messages)-1].Content, "remaining ORIGINAL") {
				t.Error("missing explicit finalization request")
			}
			if strings.Contains(system, "Inspected source") {
				t.Error("claim entered trusted prompt")
			}
			p := validPRContext("observation-3")
			p.BeforeObservationIDs = []string{"observation-2"}
			p.AfterObservationIDs = []string{"observation-3"}
			p.Relationships = []InvestigationRelationship{{From: "request", To: "danger", Relation: "source passes input to operation", Certainty: "cited", ObservationIDs: []string{"observation-3"}}}
			name = "record_pr_context"
			args = mustRecordingJSON(t, prContextRecording{ID: inv.ID, Claim: inv.Claim, PRContext: p})
		}
		message := map[string]any{"role": "assistant"}
		finish := "tool_calls"
		if n == 5 || n == 7 {
			finish = "stop"
			message["content"] = `{"findings":[],"summary":"final static fixture","coverage_notes":[]}`
		} else {
			message["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 9, MaxToolCalls: 8}}
	result, trace, err := auditor.Audit(context.Background(), snap, DiffScope{Added: map[string]map[int]bool{"service.any": {2: true}}})
	if err != nil || calls.Load() != 7 || len(result.Investigations) != 1 {
		t.Fatal(err, calls.Load(), result)
	}
	inv := result.Investigations[0]
	if inv.PRContext == nil || len(inv.PRContext.BeforeObservationIDs) != 1 || len(inv.PRContext.AfterObservationIDs) != 1 || len(inv.PRContext.Relationships) != 1 || inv.Status != "supported" {
		t.Fatal(inv)
	}
	models, tokens, patches := 0, 0, 0
	for _, tr := range trace {
		if tr.Name == "model" {
			models++
			tokens += tr.TotalTokens
		}
		if tr.Name == "record_pr_context" {
			patches++
			if tr.Error != "" {
				t.Fatal(tr)
			}
		}
	}
	if models != 7 || tokens != 70 || patches != 1 {
		t.Fatal("wrapper duplicated or lost actual usage", models, tokens, patches)
	}
	for _, note := range result.CoverageNotes {
		if strings.Contains(note, "PR impact recording gap") {
			t.Fatal(note)
		}
	}
}

func TestPrimaryRecordingFinalizationSDKKeepsTokenStopAndLastToolFence(t *testing.T) {
	for _, mode := range []string{"token_stop", "last_tool"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, _, _ := sequenceFixture()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				message := map[string]any{"role": "assistant"}
				finish := "tool_calls"
				if mode == "token_stop" && n == 2 {
					message["content"] = `{"findings":[],"summary":"early final","coverage_notes":[]}`
					finish = "stop"
				} else {
					path := "service.any"
					if n == 2 {
						path = "caller.any"
					}
					message["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]string{"name": "read_file", "arguments": fmt.Sprintf(`{"path":%q,"start":1,"end":3}`, path)}}}
				}
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
			}))
			defer server.Close()
			cfg := AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 3, MaxToolCalls: 10}
			wantCalls, wantReads := int32(3), 2
			if mode == "token_stop" {
				cfg.MaxSteps = 6
				cfg.MaxTokens = 20
				wantCalls, wantReads = 2, 1
			}
			auditor := EinoAuditor{Repository: repo, Config: cfg}
			_, trace, err := auditor.Audit(context.Background(), snap, DiffScope{Added: map[string]map[int]bool{"service.any": {2: true}}})
			if mode == "token_stop" && !errors.Is(err, ErrModelTokenBudget) {
				t.Fatal(err)
			}
			if mode == "last_tool" && !errors.Is(err, compose.ErrExceedMaxSteps) {
				t.Fatal(err)
			}
			reads, models := 0, 0
			for _, tr := range trace {
				if tr.Name == "read_file" {
					reads++
				}
				if tr.Name == "model" {
					models++
				}
			}
			if calls.Load() != wantCalls || models != int(wantCalls) || reads != wantReads {
				t.Fatal("budget fence or callback count changed", calls.Load(), models, reads, wantReads)
			}
		})
	}
}
