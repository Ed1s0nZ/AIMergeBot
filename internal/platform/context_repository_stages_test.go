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

func TestFreshStagesReadAuthorizedContextThroughSDK(t *testing.T) {
	root, snap, finding, sources := crossGitFixture(t)
	for _, stage := range []string{"verification", "synthesis"} {
		t.Run(stage, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Tools    []struct{ Function struct{ Name string } } `json:"tools"`
					Messages []struct{ Role, Content string }           `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				message := map[string]any{"role": "assistant"}
				finish := "stop"
				if calls.Add(1) == 1 {
					seen := false
					for _, tool := range request.Tools {
						if tool.Function.Name == "read_repository_file" {
							seen = true
						}
						if tool.Function.Name == "submit_finding" || tool.Function.Name == "record_hypothesis" {
							t.Error("fresh stage exposed mutation tool")
						}
					}
					if !seen {
						t.Error("fresh stage omitted context tool")
					}
					tools := []any{map[string]any{"id": "related", "type": "function", "function": map[string]string{"name": "read_repository_file", "arguments": `{"repository_id":2,"path":"guard.any","start":1,"end":3}`}}}
					if stage == "verification" {
						tools = append(tools, map[string]any{"id": "primary", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"guard.any","start":2,"end":2}`}})
					}
					message["tool_calls"], finish = tools, "tool_calls"
				} else {
					ids := []string{}
					seen := false
					for _, msg := range request.Messages {
						if msg.Role != "tool" {
							continue
						}
						var out toolOutput
						if json.Unmarshal([]byte(msg.Content), &out) != nil {
							t.Error("invalid fresh observation")
							continue
						}
						ids = append(ids, out.ObservationID)
						if out.RepositoryID == 2 && out.HeadSHA == sources[2].Snapshot.HeadSHA && strings.Contains(out.Text, "context-only needle") {
							seen = true
						}
					}
					if !seen {
						t.Error("fresh stage lost fixed source provenance")
					}
					var content []byte
					if stage == "verification" {
						content, _ = json.Marshal(verificationInput{ClaimCoverage: "full", Status: "supported", Reason: "synthetic conditional risk review", Limitations: []string{"fixture, not exploit reproduction"}, ObservationIDs: ids})
					} else {
						content, _ = json.Marshal(groupSynthesis{Summary: "synthetic fixed context summary", CoverageNotes: []string{}})
					}
					message["content"] = string(content)
				}
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
			}))
			defer server.Close()
			ctx := context.Background()
			model, err := eo.NewChatModel(ctx, &eo.ChatModelConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fresh-context-fixture"})
			if err != nil {
				t.Fatal(err)
			}
			parent := crossTools(root, snap, sources)
			result := AuditResult{Findings: []Finding{finding}, CoverageNotes: []string{}}
			e := &EinoAuditor{Config: AgentConfig{Model: "fresh-context-fixture"}}
			if stage == "verification" {
				e.verifyFindings(ctx, &result, parent, model)
			} else {
				e.synthesizeGroups(ctx, snap, &result, parent, "fixture manifest", 4, model)
			}
			if calls.Load() != 2 {
				t.Fatal("fresh stage request count", calls.Load(), result.CoverageNotes)
			}
			if stage == "verification" && (result.Findings[0].Verification == nil || result.Findings[0].Verification.Status != "supported") {
				t.Fatal("fresh verification lacked valid source anchors", result.Findings[0].Verification)
			}
			if stage == "synthesis" && result.Summary != "synthetic fixed context summary" {
				t.Fatal("synthesis lost result", result.CoverageNotes)
			}
			found := false
			for _, item := range parent.trace {
				if item.Name == "read_repository_file" {
					if item.Stage != stage {
						t.Fatal("stage trace provenance lost")
					}
					found = true
				}
			}
			if !found {
				t.Fatal("fresh context observation not persisted")
			}
		})
	}
}
