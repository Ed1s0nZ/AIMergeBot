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

func TestIndependentEinoVerificationKeepsPrimaryFindings(t *testing.T) {
	for _, mode := range []string{"supported", "rejected", "inconclusive", "forged", "invalid", "http_failure", "disabled", "clean"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, f, _ := sequenceFixture()
			var verifyCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				independent := len(request.Messages) > 0 && strings.HasPrefix(request.Messages[0].Content, "Independently review")
				message := map[string]any{"role": "assistant"}
				finish := "stop"
				if !independent {
					findings := []Finding{f}
					if mode == "clean" {
						findings = []Finding{}
					}
					raw, _ := json.Marshal(AuditResult{Findings: findings, Summary: "primary summary", CoverageNotes: []string{}})
					message["content"] = string(raw)
				} else {
					n := verifyCalls.Add(1)
					for _, tool := range request.Tools {
						if tool.Function.Name == "submit_finding" || tool.Function.Name == "update_investigation" || tool.Function.Name == "record_hypothesis" {
							t.Error("verifier exposes write tools")
						}
					}
					if mode == "http_failure" {
						w.WriteHeader(400)
						return
					}
					if n == 1 {
						finish = "tool_calls"
						message["tool_calls"] = []any{map[string]any{"id": "fresh", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"service.any","start":2,"end":2}`}}}
					} else {
						var ids []string
						for _, m := range request.Messages {
							if m.Role == "tool" {
								var out toolOutput
								if json.Unmarshal([]byte(m.Content), &out) == nil {
									ids = append(ids, out.ObservationID)
								}
							}
						}
						verdict := mode
						if mode == "forged" {
							verdict = "supported"
							ids = []string{"observation-1"}
						}
						if mode == "invalid" {
							message["content"] = "invalid"
						} else {
							raw, _ := json.Marshal(verificationInput{Status: verdict, Reason: "Synthetic fresh review, not exploit reproduction", Limitations: []string{"Synthetic model response"}, ObservationIDs: ids})
							message["content"] = string(raw)
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10}, "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
			}))
			defer server.Close()
			var checkpoints []AuditResult
			auditor := &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "verification-fixture", MaxSteps: 8, VerifyFindings: mode != "disabled", Progress: func(result AuditResult, _ []ToolTrace) error { checkpoints = append(checkpoints, result); return nil }}}
			scope := DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}}
			result, trace, err := auditor.Audit(context.Background(), snap, scope)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "clean" {
				if len(result.Findings) != 0 || verifyCalls.Load() != 0 {
					t.Fatal("clean audit verified findings")
				}
				return
			}
			if len(result.Findings) != 1 || result.Summary != "primary summary" || result.Findings[0].Confidence != "candidate" {
				t.Fatal("lost or promoted primary finding", result)
			}
			verification := result.Findings[0].Verification
			if verification == nil {
				t.Fatal("missing verification")
			}
			want := mode
			if mode == "forged" || mode == "invalid" || mode == "http_failure" {
				want = "unavailable"
			}
			if verification.Status != want {
				t.Fatalf("status %s want %s", verification.Status, want)
			}
			if mode == "disabled" && verifyCalls.Load() != 0 {
				t.Fatal("disabled phase called model")
			}
			if want == "supported" || want == "rejected" {
				if len(verification.ObservationIDs) != 1 || !strings.HasPrefix(verification.ObservationIDs[0], "verify-") {
					t.Fatal("fresh observation namespace", verification)
				}
			}
			for _, tr := range trace {
				if tr.Name == "read_file" && tr.Stage != "verification" {
					t.Fatal("fresh source stage missing")
				}
			}
			if len(checkpoints) == 0 || checkpoints[len(checkpoints)-1].Summary != "primary summary" || checkpoints[len(checkpoints)-1].Findings[0].Verification == nil {
				t.Fatal("verification not checkpointed")
			}
			if want == "inconclusive" || want == "unavailable" {
				if len(result.CoverageNotes) == 0 {
					t.Fatal("verification gap silently certified")
				}
			}
		})
	}
}

func TestVerificationCanceledBudgetRetainsFindings(t *testing.T) {
	_, snap, finding, _ := sequenceFixture()
	result := AuditResult{Findings: []Finding{finding}, Summary: "saved primary result"}
	parent := &auditTools{snap: snap}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	auditor := &EinoAuditor{}
	auditor.verifyFindings(ctx, &result, parent, nil)
	if len(result.Findings) != 1 || result.Summary != "saved primary result" || result.Findings[0].Verification.Status != "unavailable" || len(result.CoverageNotes) == 0 {
		t.Fatal("canceled verifier lost discovery or silently certified it", result)
	}
}
