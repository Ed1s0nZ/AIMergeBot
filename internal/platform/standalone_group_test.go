package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneNativeGitLargeDiffGroupsAndExclusions(t *testing.T) {
	for _, mode := range []string{"grouped", "over_budget", "excluded", "synthesis_compressed", "synthesis_compression_failed"} {
		t.Run(mode, func(t *testing.T) {
			repo, original := localGitFixture(t)
			base := original.HeadSHA
			longSynthesis := strings.HasPrefix(mode, "synthesis_")
			if longSynthesis {
				if err := os.WriteFile(filepath.Join(repo.Directory, "padding.any"), []byte(strings.Repeat(strings.Repeat("\\", 90)+"\n", 140)), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.command(context.Background(), "add", "padding.any"); err != nil {
					t.Fatal(err)
				}
				if _, err := repo.command(context.Background(), "-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "static context fixture"); err != nil {
					t.Fatal(err)
				}
				resolved, err := repo.command(context.Background(), "rev-parse", "HEAD")
				if err != nil {
					t.Fatal(err)
				}
				base = strings.TrimSpace(resolved)
			}
			lines := 2200
			if mode == "over_budget" {
				lines = 7000
			}
			data := strings.Repeat("call(input) // "+strings.Repeat("x", 60)+"\n", lines)
			if err := os.WriteFile(filepath.Join(repo.Directory, "large.any"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.command(context.Background(), "add", "large.any"); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.command(context.Background(), "-c", "user.name=fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "large static fixture"); err != nil {
				t.Fatal(err)
			}
			head, err := repo.command(context.Background(), "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			head = strings.TrimSpace(head)
			calls, synthCalls, summaries := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct {
					ResponseFormat json.RawMessage `json:"response_format"`
					Messages       []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				combined := ""
				for _, m := range req.Messages {
					combined += m.Content
				}
				if len(req.Messages) < 2 || !strings.Contains(combined, base) || !strings.Contains(combined, head) {
					t.Error("standalone snapshot not pinned")
				}
				content := `{"findings":[],"summary":"native grouped fixture","coverage_notes":[]}`
				message := map[string]any{"role": "assistant"}
				finish := "stop"
				compression := len(req.ResponseFormat) == 0 || string(req.ResponseFormat) == "null"
				if compression {
					summaries++
					if mode == "synthesis_compression_failed" {
						w.WriteHeader(400)
						return
					}
					content = "Untrusted navigation: inspect remaining grouped call context; pinned IDs unchanged."
				} else if strings.HasPrefix(req.Messages[0].Content, "Summarize a grouped") {
					synthCalls++
					content = `{"summary":"native synthesis fixture","coverage_notes":[]}`
					if longSynthesis && synthCalls < 4 {
						finish = "tool_calls"
						message["tool_calls"] = []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"padding.any","start":1,"end":140}`}}}
					}
				}
				if finish == "stop" {
					message["content"] = content
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
			}))
			defer server.Close()
			cfg := Settings{AuditTimeoutSeconds: 60}
			cfg.OpenAI.APIKey = "synthetic"
			cfg.OpenAI.Model = "synthetic"
			cfg.OpenAI.URL = server.URL
			cfg.ReAct.MaxSteps = 4
			if mode == "excluded" {
				cfg.WhitelistExtensions = []string{".any"}
			}
			run, err := AuditGit(context.Background(), repo.Directory, base, head, "", cfg)
			if err != nil {
				t.Fatal(err)
			}
			if run.BaseSHA != base || run.HeadSHA != head {
				t.Fatal("refs changed", run.Snapshot)
			}
			if mode == "excluded" {
				if calls != 0 || run.Status != "skipped" || len(run.Result.ExcludedFiles) != 1 {
					t.Fatal("exclusion did not skip model", calls, run.Status, run.Result)
				}
				return
			}
			if len(run.Result.AuditGroups) < 2 || calls != len(run.Result.AuditGroups)+synthCalls+summaries {
				t.Fatal("large diff not grouped", calls, run.Result.AuditGroups)
			}
			for _, group := range run.Result.AuditGroups {
				if group.Status != "completed" || len(group.Files) != 1 || group.Files[0] != "large.any" {
					t.Fatal("group scope lost", group)
				}
			}
			if longSynthesis {
				if summaries == 0 {
					t.Fatal("synthesis compression not exercised")
				}
				stageCalls := 0
				for _, trace := range run.Trace {
					if trace.Name == "model" && trace.Stage == "synthesis_compression" {
						stageCalls++
					}
				}
				if stageCalls != summaries {
					t.Fatal("synthesis usage missing or duplicated", stageCalls, summaries)
				}
				if mode == "synthesis_compressed" && (run.Status != "incomplete" || !hasPlanGap(run.Result.CoverageNotes) || synthCalls != 4 || run.Result.Summary != "native synthesis fixture") {
					t.Fatal("synthesis did not resume", run.Status, synthCalls, run.Result.CoverageNotes)
				}
				if mode == "synthesis_compression_failed" && (run.Status != "incomplete" || len(run.Result.CoverageNotes) == 0 || synthCalls >= 4) {
					t.Fatal("synthesis failure discarded coverage", run.Status, synthCalls)
				}
			}
			if mode == "grouped" && (run.Status != "incomplete" || len(run.Result.CoverageNotes) != 1 || !hasPlanGap(run.Result.CoverageNotes)) {
				t.Fatal("complete native diff omitted", run.Status, run.Result.CoverageNotes)
			}
			if mode == "over_budget" && (run.Status != "incomplete" || len(run.Result.CoverageNotes) == 0 || len(run.Result.AuditGroups) > 8) {
				t.Fatal("omitted native diff called clean", run.Status, run.Result.CoverageNotes)
			}
		})
	}
}
