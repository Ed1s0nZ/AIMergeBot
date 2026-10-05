package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSupplementalPhaseCompressionIsolation(t *testing.T) {
	for _, phase := range []string{"verification", "diagram"} {
		for _, fail := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "_success", true: "_failure"}[fail], func(t *testing.T) {
				repo, snap, f, diagram := sequenceFixture()
				repo.files["padding.any"] = strings.Repeat(strings.Repeat("\\", 90)+"\n", 140)
				primary, rounds, summaries := 0, 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var req struct {
						Model    string `json:"model"`
						Messages []struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						} `json:"messages"`
						ResponseFormat json.RawMessage `json:"response_format"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
					}
					compression := len(req.ResponseFormat) == 0 || string(req.ResponseFormat) == "null"
					main := !compression && strings.HasPrefix(req.Messages[0].Content, "You are a security code reviewer")
					wanted := "primary-fixture"
					if !main && phase == "verification" {
						wanted = "verifier-fixture"
					}
					if req.Model != wanted {
						t.Errorf("compression or phase used wrong model %s", req.Model)
					}
					message := map[string]any{"role": "assistant"}
					finish := "stop"
					if main {
						primary++
						raw, _ := json.Marshal(AuditResult{Findings: []Finding{f}, Summary: "retained primary result", CoverageNotes: []string{}})
						message["content"] = string(raw)
					} else if compression {
						summaries++
						if fail {
							w.WriteHeader(400)
							return
						}
						message["content"] = "Untrusted navigation: padding inspected; freshly inspect pinned service.any anchor next."
					} else {
						rounds++
						if rounds < 5 {
							finish = "tool_calls"
							args := `{"path":"padding.any","start":1,"end":140}`
							if rounds == 4 {
								args = `{"path":"service.any","start":2,"end":2}`
							}
							message["tool_calls"] = []any{map[string]any{"id": "fresh-read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": args}}}
						} else if phase == "diagram" {
							raw, _ := json.Marshal(diagram)
							message["content"] = string(raw)
						} else {
							ids := []string{}
							for _, m := range req.Messages {
								if m.Role == "tool" {
									var out toolOutput
									if json.Unmarshal([]byte(m.Content), &out) == nil && strings.Contains(out.Text, "danger(input)") {
										ids = append(ids, out.ObservationID)
									}
								}
							}
							raw, _ := json.Marshal(map[string]any{"status": "supported", "reason": "fresh pinned anchor supports the conditional fixture claim", "limitations": []string{"static fixture"}, "observation_ids": ids})
							message["content"] = string(raw)
						}
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
				}))
				defer server.Close()
				a := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "primary-fixture", VerificationModel: "verifier-fixture", MaxSteps: 4, MaxTokens: 1000, VerifyFindings: phase == "verification", GenerateDiagrams: phase == "diagram"}}
				result, trace, err := a.Audit(context.Background(), snap, BuildDiff([]Change{{NewPath: "service.any", Diff: "@@ -1,0 +2,1 @@\n+danger(input)"}}, nil, 1024))
				if err != nil || len(result.Findings) != 1 || result.Summary != "retained primary result" {
					t.Fatal("supplement affected primary", err, result)
				}
				if summaries == 0 || primary != 1 {
					t.Fatal("supplement compression never triggered", primary, rounds, summaries)
				}
				if !fail && rounds != 5 {
					t.Fatal("supplement did not continue", rounds)
				}
				if phase == "verification" {
					wanted := "supported"
					if fail {
						wanted = "unavailable"
					}
					if result.Findings[0].Verification.Status != wanted {
						t.Fatal(result.Findings[0].Verification)
					}
				} else {
					wanted := "partial"
					if fail {
						wanted = "unavailable"
					}
					if result.Findings[0].SequenceDiagram.Status != wanted {
						t.Fatal(result.Findings[0].SequenceDiagram)
					}
				}
				usage := SummarizeModelUsage(trace, ModelBudgetSettings{}, true)
				if usage.Calls != primary+rounds+summaries {
					t.Fatal("supplement usage duplicated or missing", usage.Calls, primary, rounds, summaries)
				}
				calls := 0
				for _, s := range usage.Stages {
					if s.Stage == phase+"_compression" {
						calls = s.Calls
					}
				}
				if calls != summaries {
					t.Fatal("supplement stage escaped parent trace", calls, summaries)
				}
			})
		}
	}
}

func TestVerificationCompressionPricingDoesNotUsePrimaryPrice(t *testing.T) {
	trace := []ToolTrace{{Name: "model", Stage: "verification_compression", UsageReported: true, PromptTokens: 1000000, CompletionTokens: 1000000}}
	prices := ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2, OutputPricePerMillion: 8}
	if u := SummarizeModelUsage(trace, prices, true, true); u.EstimatedCost != nil || u.EstimateUnavailableReason != "verification_price_missing" {
		t.Fatal("verifier compression priced as primary", u)
	}
	prices.VerificationPricingConfigured = true
	prices.VerificationInputPricePerMillion = 1
	prices.VerificationOutputPricePerMillion = 3
	if u := SummarizeModelUsage(trace, prices, true, true); u.EstimatedCost == nil || *u.EstimatedCost != 4 {
		t.Fatal("verifier compression price missing", u)
	}
}
