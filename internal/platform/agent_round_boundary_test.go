package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConfiguredFinalRoundCanReturnReport(t *testing.T) {
	for _, rounds := range []int{2, 4, 8} {
		t.Run(fmt.Sprint(rounds), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(request.Messages) == 0 || !strings.Contains(request.Messages[0].Content, fmt.Sprintf("decision %d of %d", n, rounds)) {
					t.Error("SDK request missing current budget")
				}
				if int(n) == rounds && !strings.Contains(request.Messages[0].Content, "do not request more tools") {
					t.Error("final request missing closing instruction")
				}
				message := map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"fixture final report","coverage_notes":[]}`}
				finish := "stop"
				if int(n) < rounds {
					finish = "tool_calls"
					message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": fmt.Sprintf("read%d", n), "type": "function", "function": map[string]string{"name": "read_file", "arguments": fmt.Sprintf(`{"path":"f%d.any","start":1,"end":1}`, n)}}}}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}, "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
			}))
			defer server.Close()
			auditor := &EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: rounds, MaxToolCalls: 80}}
			_, _, err := auditor.Audit(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
			if err != nil || int(calls.Load()) != rounds {
				t.Fatalf("configured final decision unavailable: rounds=%d HTTP=%d err=%v", rounds, calls.Load(), err)
			}
		})
	}
}
