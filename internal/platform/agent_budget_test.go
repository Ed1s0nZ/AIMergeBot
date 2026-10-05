package platform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/eino/compose"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEinoConfiguredModelRoundsAreBounded(t *testing.T) {
	for _, finishLast := range []bool{true, false} {
		t.Run(map[bool]string{true: "report_at_limit", false: "exhausted"}[finishLast], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				msg := map[string]any{"role": "assistant"}
				finish := "tool_calls"
				if finishLast && n == 4 {
					finish = "stop"
					msg["content"] = `{"findings":[],"summary":"static fixture report","coverage_notes":[]}`
				} else {
					msg["tool_calls"] = []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"file.any","start":1,"end":1}`}}}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}})
			}))
			defer server.Close()
			a := &EinoAuditor{Repository: fixtureRepo{files: map[string]string{"file.any": "safe()"}}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "synthetic", MaxSteps: 4}}
			_, _, err := a.Audit(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
			if calls.Load() != 4 {
				t.Fatal("configured rounds not respected", calls.Load())
			}
			if finishLast && err != nil {
				t.Fatal(err)
			}
			if !finishLast && !errors.Is(err, compose.ErrExceedMaxSteps) {
				t.Fatal("unbounded graph or wrong exhaustion", err)
			}
		})
	}
}
