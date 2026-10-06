package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGroupedResponseFailureRetainsTypedCause(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"summary":"incomplete fixture synthesis","coverage_notes":["group did not finish"]}`
		if calls.Add(1) == 1 {
			content = `{"findings":[],"summary":"fixture","coverage_notes":[],"unsupported":"sensitive fixture value"}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	defer server.Close()
	a := EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 4, MaxToolCalls: 80}}
	result, _, err := a.AuditGroups(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, AuditPlan{Groups: []AuditGroup{{ID: "group-1", Files: []string{"source.any"}, Scope: DiffScope{}}}})
	if err != nil || len(result.AuditGroups) != 1 || result.AuditGroups[0].Status != "failed" || result.AuditGroups[0].StopReason != "model_response_unknown_field" || len(result.CoverageNotes) == 0 {
		t.Fatalf("parse failure hidden: %+v %v", result, err)
	}
}
