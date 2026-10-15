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
	"time"
)

func TestRunnerCredentialRotationDuringDiffPreventsModelInput(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid_control", true: "rotated"}[rotate], func(t *testing.T) {
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			signal := sync.OnceFunc(func() { close(entered) })
			var repositoryRequests, modelRequests atomic.Int32
			snap := credentialAdmissionSnapshot(7)
			snap.DiffVersionID = 9
			gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				repositoryRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/versions/9") {
					signal()
					<-release
					json.NewEncoder(w).Encode(map[string]any{"head_commit_sha": snap.HeadSHA, "base_commit_sha": snap.BaseSHA, "state": "collected", "real_size": "1", "diffs": []any{map[string]any{"old_path": "main.go", "new_path": "main.go", "diff": "@@ -1 +1 @@\n-old\n+new"}}})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/repository/tree") {
					json.NewEncoder(w).Encode([]any{map[string]any{"id": strings.Repeat("c", 40), "name": "main.go", "path": "main.go", "type": "blob", "mode": "100644"}})
					return
				}
				t.Errorf("unexpected repository request: %s", r.URL.Path)
				w.WriteHeader(404)
			}))
			defer gitlab.Close()
			defer unblock()
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 3, "total_tokens": 6}, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"controlled static audit","coverage_notes":[]}`}}}})
			}))
			defer model.Close()
			svc, s := activationFixture(t, credentialActivationOptions{})
			cfg := svc.Snapshot()
			cfg.GitLab.URL, cfg.OpenAI.URL = gitlab.URL, model.URL
			cfg.OpenAI.APIKey, cfg.OpenAI.Model, cfg.ReAct.Model = "synthetic", "fixture", "fixture"
			cfg.GitAudit.Enabled, cfg.VerifyFindings, cfg.GenerateSequenceDiagrams = false, false, false
			if err := svc.Save(cfg); err != nil {
				t.Fatal(err)
			}
			ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			snap.AuditPolicy = capturePolicy(svc.Snapshot())
			id, _, err := s.Enqueue(ctx, snap, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Checkpoint(ctx, id, AuditResult{Summary: "RETAINED-CHECKPOINT"}, nil); err != nil {
				t.Fatal(err)
			}
			runner := &Runner{Store: s, Settings: svc, Timeout: 10 * time.Second}
			go func() { runner.execute(ctx, id); close(done) }()
			defer func() { unblock(); <-done }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("runner never requested pinned diff")
			}
			if rotate {
				cfg = svc.Snapshot()
				cfg.GitLab.Token = "PRIVATE-B"
				if err = svc.Save(cfg); err != nil {
					t.Fatal(err)
				}
			}
			unblock()
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("runner did not stop")
			}
			var status, result string
			if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=?`, id).Scan(&status, &result); err != nil {
				t.Fatal(err)
			}
			if rotate {
				if modelRequests.Load() != 0 || repositoryRequests.Load() != 1 || status != "cancelled" || !strings.Contains(result, "RETAINED-CHECKPOINT") {
					t.Fatal("revoked diff reached model or replaced checkpoint", status, repositoryRequests.Load(), modelRequests.Load())
				}
			} else if modelRequests.Load() == 0 || (status != "succeeded" && status != "incomplete") {
				t.Fatal("valid control did not reach real model SDK", status, modelRequests.Load())
			}
		})
	}
}
