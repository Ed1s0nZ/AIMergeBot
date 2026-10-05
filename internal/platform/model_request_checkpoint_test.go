package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestModelRequestCheckpointPrecedesHTTPAndCompletionReplacesIt(t *testing.T) {
	var persisted atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !persisted.Load() {
			t.Error("HTTP request sent before pending checkpoint")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"static report","coverage_notes":[]}`}}}})
	}))
	defer server.Close()
	auditor := &EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxTokens: 100, Progress: func(_ AuditResult, trace []ToolTrace) error {
		for _, tr := range trace {
			if tr.Name == "model" && tr.Error == pendingModelUsage {
				persisted.Store(true)
			}
		}
		return nil
	}}}
	_, trace, err := auditor.Audit(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
	if err != nil || calls.Load() != 1 {
		t.Fatal(err)
	}
	modelCount := 0
	for _, tr := range trace {
		if tr.Name == "model" {
			modelCount++
			if tr.Error != "" || tr.TotalTokens != 20 {
				t.Fatal("pending checkpoint not replaced")
			}
		}
	}
	if modelCount != 1 {
		t.Fatal("duplicate usage count", modelCount)
	}
}
func TestModelRequestNotSentWhenCheckpointFails(t *testing.T) {
	for _, failure := range []error{ErrConflict, errors.New("private checkpoint failed")} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
		auditor := &EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxTokens: 100, Progress: func(AuditResult, []ToolTrace) error { return failure }}}
		_, _, err := auditor.Audit(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
		server.Close()
		if err == nil || calls.Load() != 0 {
			t.Fatal("model billed after checkpoint failure", calls.Load(), err)
		}
	}
}
