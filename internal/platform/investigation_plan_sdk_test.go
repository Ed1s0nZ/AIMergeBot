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
	for _, mode := range []string{"unplanned", "planned", "recover", "recover_id"} {
		planned := mode != "unplanned"
		recoverPlan := mode == "recover" || mode == "recover_id"
		recoverID := mode == "recover_id"
		repo, snap, _, _ := sequenceFixture()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Tools []struct {
					Function struct {
						Name       string         `json:"name"`
						Parameters map[string]any `json:"parameters"`
					} `json:"function"`
				} `json:"tools"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			n := calls.Add(1)
			for _, tool := range req.Tools {
				if tool.Function.Name == "update_investigation" {
					assertAssessmentSchema(t, tool.Function.Parameters)
				}
			}
			if len(req.Messages) == 0 || !strings.Contains(req.Messages[0].Content, "Server-owned recording progress") {
				t.Error("SDK missing current progress navigation")
			}
			if planned && n == 3 && !strings.Contains(req.Messages[0].Content, `"ledger_count":1`) {
				t.Error("SDK progress did not refresh after recording")
			}
			if planned && n == 3 && (!strings.Contains(req.Messages[0].Content, `"unresolved_ledger_count":1`) || !strings.Contains(req.Messages[0].Content, "record_hypothesis always creates investigating")) {
				t.Error("SDK omitted unresolved hypothesis closure guidance")
			}
			if planned && ((!recoverPlan && n == 4) || (recoverPlan && n == 5)) && !strings.Contains(req.Messages[0].Content, `"unresolved_ledger_count":0`) {
				t.Error("SDK unresolved count did not refresh after explicit resolution")
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
			} else if planned && (n == 3 || (recoverPlan && n == 4)) {
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
				if recoverPlan && n == 4 {
					seen := false
					for _, m := range req.Messages {
						var out toolOutput
						if m.Role == "tool" && json.Unmarshal([]byte(m.Content), &out) == nil && out.Error != "" {
							seen = true
							if out.EvidenceEligible || (recoverID && !strings.Contains(out.Error, `known_investigation_ids=["plan"]`)) || (!recoverID && (!strings.Contains(out.Error, `"question":"Inspect input_control"`) || !strings.Contains(out.Error, `"question":"Inspect outcome"`))) {
								t.Error("SDK missing exact non-source identities", out)
							}
							if strings.Contains(req.Messages[0].Content, "expected_task_identities=") || strings.Contains(req.Messages[0].Content, "known_investigation_ids=") {
								t.Error("untrusted identity inserted in system")
							}
						}
					}
					if !seen {
						t.Error("SDK missing conflict feedback")
					}
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
				if recoverPlan && !recoverID && n == 3 {
					plan[0].Question = "paraphrased input"
					plan[3].Question = "paraphrased outcome"
				}
				updateID := "plan"
				if recoverID && n == 3 {
					updateID = ""
				}
				toolCall("update_investigation", investigationAssessmentUpdate{Investigation: Investigation{ID: updateID, Claim: "static candidate examined", Counterevidence: []string{"fixture judgment"}, CounterObservationIDs: []string{id}, Plan: plan}, ClaimAssessment: "evidence_refutes_claim"})
			} else if recoverPlan && n == 5 {
				toolCall("resolve_recording_errors", recordingCorrectionsArgs{Corrections: []recordingCorrection{{"observation-4", "observation-5"}}})
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
		if recoverPlan {
			if calls.Load() != 6 {
				t.Fatal("unexpected SDK round count", calls.Load())
			}
			badRetained := false
			for _, tr := range trace {
				if tr.ObservationID == "observation-4" && ((recoverID && strings.Contains(tr.Error, "known_investigation_ids=")) || (!recoverID && strings.Contains(tr.Error, "expected_task_identities="))) {
					badRetained = true
				}
			}
			if !badRetained {
				t.Fatal("original failed trace erased")
			}
		}
		if recoverID {
			resolverRejected := false
			for _, tr := range trace {
				if tr.Name == "resolve_recording_errors" && tr.Error != "" {
					resolverRejected = true
				}
			}
			if !resolverRejected {
				t.Fatal("cross-identity resolver allowed to retire error")
			}
			if len(result.Investigations) != 1 || result.Investigations[0].Status != "rejected" || hasPlanGap(result.CoverageNotes) || !strings.Contains(strings.Join(result.CoverageNotes, " "), "Tool failed: update_investigation") {
				t.Fatal("unknown identity error was hidden or legitimate selected update failed", result)
			}
		} else if planned {
			if len(result.CoverageNotes) != 1 || !strings.Contains(result.CoverageNotes[0], "PR impact recording gap") || hasPlanGap(result.CoverageNotes) || len(result.Investigations) != 1 || result.Investigations[0].Status != "rejected" || len(trace) < 4 {
				t.Fatal("planned recording not preserved", result)
			}
		} else if len(result.CoverageNotes) != 1 || !hasPlanGap(result.CoverageNotes) {
			t.Fatal("no ledger called complete", result)
		}
	}
}
