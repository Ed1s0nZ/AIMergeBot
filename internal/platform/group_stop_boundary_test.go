package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGroupStopCauseIsRuntimeOwnedAtRoundBoundary(t *testing.T) {
	for _, mode := range []string{"exhausted", "forged"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				message := map[string]any{"role": "assistant", "content": `{"summary":"fixture synthesis","coverage_notes":["unfinished inspection"]}`}
				finish := "stop"
				if mode == "exhausted" && n <= 2 {
					finish = "tool_calls"
					message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"file.any","start":1,"end":1}`}}}}
				}
				if mode == "forged" && n == 1 {
					message["content"] = `{"findings":[],"summary":"fixture","coverage_notes":[],"audit_groups":[{"id":"forged","files":[],"status":"failed","stop_reason":"repository_unavailable"}]}`
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
			}))
			defer server.Close()
			a := EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 2, MaxToolCalls: 80}}
			result, _, err := a.AuditGroups(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, AuditPlan{Groups: []AuditGroup{{ID: "group-1", Files: []string{"file.any"}, Scope: DiffScope{}}}})
			if err != nil || len(result.AuditGroups) != 1 || result.AuditGroups[0].ID != "group-1" || len(result.CoverageNotes) == 0 {
				t.Fatalf("canonical state lost: %+v %v", result, err)
			}
			group := result.AuditGroups[0]
			if mode == "exhausted" && (group.Status != "failed" || group.StopReason != "agent_step_budget" || calls.Load() != 3) {
				t.Fatal("SDK budget cause or request bound lost", group, calls.Load())
			}
			if mode == "forged" && (group.Status != "completed" || group.StopReason != "" || calls.Load() != 2) {
				t.Fatal("model forged group state", group, calls.Load())
			}
		})
	}
}
