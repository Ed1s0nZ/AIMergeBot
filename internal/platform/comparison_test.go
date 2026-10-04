package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	legacy "pr_agent/internal"
)

// This is a deterministic regression comparison, not a live model accuracy benchmark.
func TestLegacyCleanResponseComparison(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		content := `{"findings":[],"summary":"No issue supported by evidence","coverage_notes":[]}`
		if req.Model == "legacy-fixture" {
			content = `{"thought":"analysis complete","final_analysis":{"summary":"No issue supported by evidence","issues":[],"risk_level":"low","recommendations":[]}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120}})
	}))
	defer server.Close()
	old := legacy.NewReActAuditorWithConfig("fixture", server.URL, "legacy-fixture", nil, 1, "simplified", 4, 0.1, 1, false)
	start := time.Now()
	before, err := old.AuditWithReAct("File: a.go\n@@ -0,0 +1 @@\n+safe()", map[string]interface{}{"project_id": 1})
	if err != nil {
		t.Fatal(err)
	}
	oldDuration := time.Since(start)
	scope := BuildDiff([]Change{{NewPath: "a.go", Diff: "@@ -0,0 +1 @@\n+safe()"}}, nil, 10000)
	next := EinoAuditor{Repository: fixtureRepo{files: map[string]string{"a.go": "safe()"}}, Config: AgentConfig{APIKey: "fixture", BaseURL: server.URL, Model: "new-fixture", MaxSteps: 4}}
	start = time.Now()
	after, trace, err := next.Audit(context.Background(), Snapshot{HeadSHA: "fixture"}, scope)
	newDuration := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Issues) != 1 || len(after.Findings) != 0 {
		t.Fatalf("clean-case regression: old=%d new=%d", len(before.Issues), len(after.Findings))
	}
	reported := false
	for _, tr := range trace {
		if tr.UsageReported && tr.PromptTokens == 100 && tr.CompletionTokens == 20 {
			reported = true
		}
	}
	if !reported {
		t.Fatal("model token usage callback missing")
	}
	t.Logf("fixture only: clean-case legacy false-positive=%d Eino false-positive=%d; legacy=%s Eino=%s; reported fixture tokens=120 (not actual production cost)", len(before.Issues), len(after.Findings), oldDuration, newDuration)
}
