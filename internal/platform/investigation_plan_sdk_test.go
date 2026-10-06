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

func TestEinoPlanRequiresLedgerBeforeCleanCompletion(t *testing.T) {
	for _, planned := range []bool{false, true} {
		repo, snap, _, _ := sequenceFixture()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			n := calls.Add(1)
			if len(req.Messages) == 0 || !strings.Contains(req.Messages[0].Content, "Server-owned recording progress") {
				t.Error("SDK missing current progress navigation")
			}
			if planned && n == 3 && !strings.Contains(req.Messages[0].Content, `"ledger_count":1`) {
				t.Error("SDK progress did not refresh after recording")
			}
			message := map[string]any{"role": "assistant"}
			finish := "stop"
			toolCall := func(name string, args any) {
				raw, _ := json.Marshal(args)
				finish = "tool_calls"
				message["tool_calls"] = []any{map[string]any{"id": name, "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}}}
			}
			if planned && n == 1 {
				toolCall("read_file", readArgs{Path: "service.any", Start: 1, End: 2})
			} else if planned && n == 2 {
				toolCall("record_hypothesis", Investigation{ID: "plan", Claim: "static candidate examined", Plan: pendingPlan()})
			} else if planned && n == 3 {
				seenGap := false
				for _, m := range req.Messages {
					var out toolOutput
					if m.Role == "tool" && json.Unmarshal([]byte(m.Content), &out) == nil {
						for _, gap := range out.RecordingGaps {
							if gap == "plan_unfinished" {
								seenGap = true
								if out.EvidenceEligible {
									t.Error("SDK recording feedback is source evidence")
								}
							}
						}
					}
				}
				if !seenGap {
					t.Error("SDK did not receive timely plan gap")
				}
				id := ""
				for _, m := range req.Messages {
					var out toolOutput
					if m.Role == "tool" && json.Unmarshal([]byte(m.Content), &out) == nil && out.EvidenceEligible {
						id = out.ObservationID
					}
				}
				plan := pendingPlan()
				for i := range plan {
					plan[i].Status = "checked"
					plan[i].Reason = "synthetic counterevidence judgment, not semantic proof"
					plan[i].ObservationIDs = []string{id}
				}
				toolCall("update_investigation", Investigation{ID: "plan", Claim: "static candidate examined", Status: "rejected", Counterevidence: []string{"fixture judgment"}, CounterObservationIDs: []string{id}, Plan: plan})
			} else {
				message["content"] = `{"findings":[],"summary":"static fixture, not safety certification","coverage_notes":[]}`
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
		}))
		a := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 8}}
		result, trace, err := a.Audit(context.Background(), snap, DiffScope{})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if planned {
			if len(result.CoverageNotes) != 0 || len(result.Investigations) != 1 || result.Investigations[0].Status != "rejected" || len(trace) < 4 {
				t.Fatal("planned recording not preserved", result)
			}
		} else if len(result.CoverageNotes) != 1 || !hasPlanGap(result.CoverageNotes) {
			t.Fatal("no ledger called complete", result)
		}
	}
}
