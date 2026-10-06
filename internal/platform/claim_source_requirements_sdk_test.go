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
)

func TestClaimSourceRequirementsSDKUsesFixedPolicyAndFreshContext(t *testing.T) {
	for _, includeContext := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "missing_context"}[includeContext], func(t *testing.T) {
			repo, snap, _, sources := crossGitFixture(t)
			parent := crossTools(repo, snap, sources)
			item := Investigation{ID: "context-claim", Claim: "PRIVATE_CLAIM: changed input reaches the configured contract", Status: "supported", Evidence: []string{"PRIMARY_SECRET"}}
			before, _ := json.Marshal(item)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				n := requests.Add(1)
				ids := []string{}
				for _, m := range req.Messages {
					if m.Role == "system" && (strings.Contains(m.Content, "PRIVATE_CLAIM") || strings.Contains(m.Content, "guard.any") || strings.Contains(m.Content, "PRIMARY_SECRET")) {
						t.Error("untrusted review data promoted to system")
					}
					if m.Role == "user" {
						var payload struct {
							Requirements claimSourceRequirements `json:"source_requirements"`
						}
						if json.Unmarshal([]byte(m.Content), &payload) != nil || !reflect.DeepEqual(payload.Requirements.ContextRepositoryIDs, []int{2}) || !payload.Requirements.FreshCitationsRequired || payload.Requirements.PrimaryChangedSource == "" {
							t.Error("missing policy-aligned source requirement", m.Content)
						}
						if strings.Contains(m.Content, "PRIMARY_SECRET") || strings.Contains(m.Content, `"status"`) {
							t.Error("primary judgment leaked")
						}
					}
					if m.Role == "tool" {
						var out toolOutput
						if json.Unmarshal([]byte(m.Content), &out) != nil || !out.EvidenceEligible {
							t.Error("fresh read unavailable", m.Content)
						} else {
							ids = append(ids, out.ObservationID)
						}
					}
				}
				msg := map[string]any{"role": "assistant"}
				finish := "stop"
				if n == 1 {
					calls := []any{}
					add := func(name string, a any) {
						raw, _ := json.Marshal(a)
						calls = append(calls, map[string]any{"id": name + string(raw), "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}})
					}
					add("read_files", batchArgs{Files: []readArgs{{Path: "guard.any", Base: true, Start: 1, End: 3}, {Path: "guard.any", Start: 1, End: 3}}})
					if includeContext {
						add("read_repository_file", contextReadArgs{RepositoryID: 2, Path: "guard.any", Start: 1, End: 3})
					}
					msg["tool_calls"] = calls
					finish = "tool_calls"
				} else {
					raw, _ := json.Marshal(claimVerificationInput{Verdict: "true", Reason: "Synthetic source coverage proposal, not semantic accuracy proof", Limitations: []string{}, ObservationIDs: ids})
					msg["content"] = string(raw)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
			}))
			defer server.Close()
			model, err := eo.NewChatModel(context.Background(), &eo.ChatModelConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "source-requirement-fixture"})
			if err != nil {
				t.Fatal(err)
			}
			e := EinoAuditor{Config: AgentConfig{Model: "source-requirement-fixture", VerifyFindings: true}}
			result := AuditResult{Investigations: []Investigation{item}}
			pool := newVerificationBudget(context.Background())
			defer pool.cancel()
			e.verifyInvestigationClaims(context.Background(), &result, parent, model, pool)
			v := result.Investigations[0].ClaimVerification
			wantStatus, wantVerdict, wantCalls := "consistent", "true", 4 // Batch wrapper + two primary reads + one context read.
			if !includeContext {
				wantStatus, wantVerdict, wantCalls = "inconclusive", "unknown", 3 // The primary batch charges its wrapper and both children.
			}
			if v == nil || v.Status != wantStatus || v.Verdict != wantVerdict || requests.Load() != 2 || pool.remaining != 40-wantCalls {
				t.Fatal("source gate or original shared budget changed", v, requests.Load(), pool.remaining)
			}
			copy := result.Investigations[0]
			copy.ClaimVerification = nil
			after, _ := json.Marshal(copy)
			if string(before) != string(after) {
				t.Fatal("primary record changed")
			}
			for _, id := range v.ObservationIDs {
				if !strings.HasPrefix(id, claimObservationPrefix(item)+"-") {
					t.Fatal("inherited source ID", id)
				}
			}
		})
	}
}

func TestRequiredClaimContextIDsAreDeterministicAndDoNotNormalizePolicy(t *testing.T) {
	snap := Snapshot{AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 9}, {ProjectID: 2}, {ProjectID: 9}, {ProjectID: 0}}}}
	before, _ := json.Marshal(snap)
	if got := requiredClaimContextIDs(snap); !reflect.DeepEqual(got, []int{0, 2, 9}) {
		t.Fatal(got)
	}
	after, _ := json.Marshal(snap)
	if string(before) != string(after) {
		t.Fatal("fixed policy mutated")
	}
	if got := requiredClaimContextIDs(Snapshot{}); got == nil || len(got) != 0 {
		t.Fatal("empty model requirements must be JSON array", got)
	}
}
