package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	eo "github.com/cloudwego/eino-ext/components/model/openai"
)

type claimUnavailableRepo struct{ Repository }

func (r claimUnavailableRepo) ReadFile(context.Context, Snapshot, string, bool) (string, error) {
	return "", ErrRepositoryUnavailable
}

func TestIndependentClaimReviewSDKPreservesPrimaryAndFreshSources(t *testing.T) {
	for _, mode := range []string{"disagreed", "consistent", "unknown", "forged", "http_failure", "exhausted", "checkpoint", "investigating", "disabled", "alternate", "finding_first", "source_unavailable"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, f, _ := sequenceFixture()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Model    string
					Messages []struct{ Role, Content string }
					Tools    []struct{ Function struct{ Name string } }
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				for _, m := range req.Messages {
					if m.Role == "user" && (strings.Contains(m.Content, "FIRST_REVIEW_SECRET") || strings.Contains(m.Content, `"status"`) || strings.Contains(m.Content, "counterevidence")) {
						t.Error("primary judgment leaked into verifier", m.Content)
					}
				}
				for _, tool := range req.Tools {
					if tool.Function.Name == "submit_finding" || tool.Function.Name == "record_hypothesis" || tool.Function.Name == "update_investigation" || tool.Function.Name == "resolve_recording_errors" {
						t.Error("mutation tool exposed")
					}
				}
				wantModel := "claim-fixture"
				if mode == "alternate" {
					wantModel = "claim-review"
				}
				if req.Model != wantModel {
					t.Error("wrong verification model", req.Model)
				}
				n := requests.Add(1)
				if mode == "http_failure" {
					w.WriteHeader(400)
					return
				}
				msg := map[string]any{"role": "assistant"}
				finish := "stop"
				if n == 1 {
					finish = "tool_calls"
					calls := []any{}
					for _, base := range []bool{true, false} {
						args, _ := json.Marshal(readArgs{Path: f.File, Base: base, Start: 1, End: 2})
						calls = append(calls, map[string]any{"id": string(args), "type": "function", "function": map[string]string{"name": "read_file", "arguments": string(args)}})
					}
					msg["tool_calls"] = calls
				} else {
					ids := []string{}
					for _, m := range req.Messages {
						if m.Role == "tool" {
							var out toolOutput
							if json.Unmarshal([]byte(m.Content), &out) == nil {
								ids = append(ids, out.ObservationID)
							}
						}
					}
					verdict := "true"
					if mode == "consistent" {
						verdict = "false"
					}
					if mode == "unknown" {
						verdict = "unknown"
					}
					if mode == "forged" {
						ids = []string{"observation-1"}
					}
					raw, _ := json.Marshal(claimVerificationInput{Verdict: verdict, Reason: "controlled SDK fixture judgment, not accuracy proof", Limitations: []string{}, ObservationIDs: ids})
					msg["content"] = string(raw)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
			}))
			defer server.Close()
			model, err := eo.NewChatModel(context.Background(), &eo.ChatModelConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "claim-fixture"})
			if err != nil {
				t.Fatal(err)
			}
			e := EinoAuditor{Config: AgentConfig{Model: "claim-fixture", BaseURL: server.URL, APIKey: "synthetic", VerifyFindings: true}}
			if mode == "alternate" {
				e.Config.VerificationModel = "claim-review"
			}
			parent := &auditTools{repo: repo, snap: snap, scope: DiffScope{Included: []string{f.File}}, cache: map[string]string{}}
			if mode == "source_unavailable" {
				parent.repo = claimUnavailableRepo{repo}
			}
			if mode == "checkpoint" {
				parent.progress = func(AuditResult, []ToolTrace) error { return ErrConflict }
			}
			item := Investigation{ID: "group-a-local", Claim: "HEAD remains compatible", Status: "rejected", Evidence: []string{"FIRST_REVIEW_SECRET"}, Counterevidence: []string{"primary recorded judgment"}}
			if mode == "investigating" {
				item.Status = "investigating"
			}
			before, _ := json.Marshal(item)
			result := AuditResult{Investigations: []Investigation{item}, Summary: "original summary", Findings: []Finding{}}
			pool := newVerificationBudget(context.Background())
			defer pool.cancel()
			if mode == "exhausted" {
				pool.remaining = 0
			}
			if mode == "finding_first" {
				pool.remaining = 2
				result.Findings = []Finding{f}
				e.verifyFindingsWithBudget(context.Background(), &result, parent, model, pool)
			}
			if mode == "disabled" {
				e.Config.VerifyFindings = false
				e.supplement(context.Background(), snap, &result, parent, nil, model, "")
			} else {
				e.verifyInvestigationClaims(context.Background(), &result, parent, model, pool)
			}
			v := result.Investigations[0].ClaimVerification
			want := "disagreed"
			switch mode {
			case "consistent":
				want = "consistent"
			case "unknown":
				want = "inconclusive"
			case "forged", "http_failure", "exhausted", "checkpoint", "finding_first", "source_unavailable":
				want = "unavailable"
			case "disabled":
				want = "disabled"
			}
			if mode == "investigating" {
				if v != nil || requests.Load() != 0 {
					t.Fatal("unresolved investigation reviewed", v)
				}
				return
			}
			if v == nil || v.Status != want || v.AssessedClaim != item.Claim || result.Summary != "original summary" {
				t.Fatal(mode, v, result)
			}
			copy := result.Investigations[0]
			copy.ClaimVerification = nil
			after, _ := json.Marshal(copy)
			if string(before) != string(after) {
				t.Fatal("primary investigation modified")
			}
			if mode == "exhausted" || mode == "checkpoint" || mode == "disabled" {
				if requests.Load() != 0 {
					t.Fatal("forbidden model request", requests.Load())
				}
			} else if mode == "source_unavailable" {
				if requests.Load() != 1 || !parent.repositoryUnavailable {
					t.Fatal("executor failure did not stop further HTTP", requests.Load())
				}
			} else if mode != "http_failure" && requests.Load() != 2 {
				t.Fatal("unexpected request count", requests.Load())
			}
			if want != "consistent" && want != "disabled" && len(result.CoverageNotes) == 0 {
				t.Fatal("unverified review lost coverage")
			}
			if want == "disagreed" || want == "consistent" {
				if len(v.ObservationIDs) != 2 || !strings.HasPrefix(v.ObservationIDs[0], "claim-") || pool.remaining != 38 {
					t.Fatal("fresh provenance/shared debit", v, pool.remaining)
				}
			}
		})
	}
}
