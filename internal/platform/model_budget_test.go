package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Exercises real Eino/OpenAI binding and multiple tool/model rounds, not a
// stand-in accounting callback. A budget must survive WithTools.
func TestEinoTokenBudgetStopsNextRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage any
		limit int
		want  error
	}{
		{"threshold", map[string]int{"prompt_tokens": 6, "completion_tokens": 4, "total_tokens": 10}, 10, ErrModelTokenBudget},
		{"missing", nil, 10, ErrModelUsageUnknown},
		{"overshoot", map[string]int{"prompt_tokens": 16, "completion_tokens": 4, "total_tokens": 20}, 10, ErrModelTokenBudget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": tc.usage, "choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"file.any","start":1,"end":1}`}}}}}}})
			}))
			defer server.Close()
			a := &EinoAuditor{Repository: fixtureRepo{files: map[string]string{"file.any": "safe()"}}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "synthetic", MaxSteps: 4, MaxTokens: tc.limit}}
			var checkpoint AuditResult
			a.Config.Progress = func(result AuditResult, _ []ToolTrace) error { checkpoint = result; return nil }
			result, trace, err := a.Audit(context.Background(), Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{Notes: []string{"selected-file follow-up scope"}, Excluded: []string{"readme.md"}})
			if len(result.ExcludedFiles) != 1 || len(result.CoverageNotes) < 2 || result.CoverageNotes[0] != "selected-file follow-up scope" || len(checkpoint.ExcludedFiles) != 1 || len(checkpoint.CoverageNotes) < 2 || checkpoint.CoverageNotes[0] != "selected-file follow-up scope" {
				t.Fatal("interruption discarded scope limits")
			}

			if !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
			models := 0
			for _, tr := range trace {
				if tr.Name == "model" {
					models++
				}
			}
			if models != 1 {
				t.Fatalf("duplicate or missing model usage: %d traces=%+v", models,trace)
			}
			if len(trace) == 0 || len(result.CoverageNotes) == 0 {
				t.Fatal("interrupted audit evidence lost")
			}
		})
	}
}
func TestTokenBudgetContextSharedAndIsolated(t *testing.T) {
	parent := withModelBudget(context.Background(), 10)
	child := withModelBudget(parent, 100)
	if parent.Value(modelBudgetKey{}) != child.Value(modelBudgetKey{}) {
		t.Fatal("child reset budget")
	}
	other := withModelBudget(context.Background(), 10)
	if other.Value(modelBudgetKey{}) == parent.Value(modelBudgetKey{}) {
		t.Fatal("budget leaked across tasks")
	}
}

func TestModelBudgetSettingsRoundTripAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `listen: 127.0.0.1:1234
openai:
  url: https://model.example/v1
  model: fixture
  api_key: synthetic-local-only
gitlab:
  url: https://git.example
model_budget:
  max_tokens: 50000
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := OpenSettings(path, path)
	if err != nil {
		t.Fatal(err)
	}
	before := capturePolicy(svc.Snapshot())
	public := svc.Public()
	encoded, _ := json.Marshal(public)
	next, err := svc.DecodePublic(encoded)
	if err != nil {
		t.Fatal(err)
	}
	next.VerificationModel = "independent-fixture"
	next.ModelBudget.MaxTokens = 100000
	next.ModelBudget.Currency = "USD"
	next.ModelBudget.InputPricePerMillion = 2
	next.ModelBudget.OutputPricePerMillion = 8
	if err = svc.Save(next); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenSettings(path, path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := reloaded.Snapshot()
	if cfg.VerificationModel != "independent-fixture" || cfg.ModelBudget.MaxTokens != 100000 || cfg.ModelBudget.Currency != "USD" || cfg.ModelBudget.InputPricePerMillion != 2 || cfg.ModelBudget.OutputPricePerMillion != 8 || cfg.OpenAI.APIKey != "synthetic-local-only" {
		t.Fatal("budget or secret retention lost")
	}
	if policyDigest(before) == policyDigest(capturePolicy(cfg)) {
		t.Fatal("budget not frozen in identity")
	}
	delete(public, "model_budget")
	delete(public, "verification_model")
	encoded, _ = json.Marshal(public)
	next, err = svc.DecodePublic(encoded)
	if err != nil || next.VerificationModel != "independent-fixture" || next.ModelBudget.MaxTokens != 100000 {
		t.Fatal("old client reset budget", err)
	}
	for _, n := range []int{-1, 10000001} {
		next.ModelBudget.MaxTokens = n
		if validateSettings(next) == nil {
			t.Fatal("accepted invalid budget")
		}
	}
}
