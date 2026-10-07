package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func formattingScopeAuditFixture(t *testing.T) (fixtureRepo, Snapshot, DiffScope) {
	t.Helper()
	repo := fixtureRepo{files: map[string]string{"service.any": "func main() {\n        run()\n}\n"}}
	snap := Snapshot{BaseSHA: "base-commit", HeadSHA: "head-commit"}
	scope := BuildDiff([]Change{{NewPath: "service.any", OldPath: "service.any", Diff: "@@ -1,3 +1,3 @@\n func main() {\n-    run()\n+        run()\n }"}}, nil, 96*1024)
	return repo, snap, scope
}

func TestFormattingScopeFindingThroughAuditSDK(t *testing.T) {
	for _, mode := range []string{"enabled", "disabled", "verification-disabled"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, scope := formattingScopeAuditFixture(t)
			var calls atomic.Int32
			var promptMu sync.Mutex
			systemPrompt := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var req struct {
					Messages []struct{ Role, Content string }
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				system := ""
				for _, m := range req.Messages {
					if m.Role == "system" {
						system += m.Content
					}
				}
				promptMu.Lock()
				systemPrompt = system
				promptMu.Unlock()
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"clean fixture","coverage_notes":[]}`}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
			}))
			defer server.Close()
			cfg := AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 3, VerifyFindings: true, GenerateDiagrams: true, CheckFormattingScope: mode != "disabled"}
			if mode == "verification-disabled" {
				cfg.VerifyFindings = false
			}
			auditor := EinoAuditor{Repository: repo, Config: cfg}
			result, _, err := auditor.Audit(context.Background(), snap, scope)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "disabled" {
				if len(result.Findings) != 0 {
					t.Fatalf("disabled setting must not add findings: %+v", result.Findings)
				}
			} else {
				if len(result.Findings) != 1 {
					t.Fatalf("expected one deterministic finding: %+v", result.Findings)
				}
				f := result.Findings[0]
				if f.Origin != deterministicFormattingOrigin || f.File != "service.any" || f.Line != 2 || f.Evidence != "        run()" {
					t.Fatalf("unexpected deterministic finding: %+v", f)
				}
				if f.Verification != nil {
					t.Fatalf("deterministic finding must not enter security verification: %+v", f.Verification)
				}
				if f.SequenceDiagram == nil || f.SequenceDiagram.Status != "unavailable" || !strings.Contains(f.SequenceDiagram.Reason, "确定性") {
					t.Fatalf("deterministic finding must not enter diagram generation: %+v", f.SequenceDiagram)
				}
				if notes := strings.Join(result.CoverageNotes, ";"); strings.Contains(notes, "PR impact recording gap") {
					t.Fatalf("deterministic finding must not add PR recording gaps: %v", result.CoverageNotes)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("supplemental phases must not call the model for deterministic findings: %d calls", calls.Load())
			}
			promptMu.Lock()
			system := systemPrompt
			promptMu.Unlock()
			announced := strings.Contains(system, "deterministic low-severity formatting-scope findings")
			if mode == "disabled" && announced {
				t.Fatal("disabled setting must not announce deterministic findings")
			}
			if mode != "disabled" && !announced {
				t.Fatal("enabled setting must announce deterministic findings to the model")
			}
		})
	}
}
