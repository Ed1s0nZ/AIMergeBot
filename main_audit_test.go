package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pr_agent/internal/platform"
)

func TestNativeAuditCLIReportsIncompleteWithNonzeroExit(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "aimangebot")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %v %s", err, out)
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Git fixture: %v %s", err, out)
		}
		return string(out)
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.com")
	if err := os.WriteFile(filepath.Join(repo, "source.any"), []byte("safe()\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "source.any")
	git("commit", "-m", "base fixture")
	if err := os.WriteFile(filepath.Join(repo, "source.any"), []byte("changed()\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "source.any")
	git("commit", "-m", "head fixture")
	for _, mode := range []string{"completed", "incomplete", "failed"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "failed" {
					w.WriteHeader(400)
					return
				}
				notes := []string{}
				if mode == "incomplete" {
					notes = []string{"caller context unavailable"}
				}
				raw, _ := json.Marshal(platform.AuditResult{Findings: []platform.Finding{}, Summary: "static CLI fixture", CoverageNotes: notes})
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": string(raw)}}}})
			}))
			defer server.Close()
			runtime := t.TempDir()
			config := []byte("gitlab:\n  url: https://gitlab.com\nopenai:\n  url: " + server.URL + "\n  api_key: synthetic\n  model: synthetic\naudit_timeout_seconds: 30\nverify_findings: false\ngenerate_sequence_diagrams: false\n")
			if err := os.WriteFile(filepath.Join(runtime, "config.yaml"), config, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "-audit-repo", repo, "-base", "HEAD~1", "-head", "HEAD")
			cmd.Dir = runtime
			stdout, err := cmd.Output()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			want := map[string]int{"completed": 0, "incomplete": 2, "failed": 1}[mode]
			if code != want {
				var stderr []byte
				if exit, ok := err.(*exec.ExitError); ok {
					stderr = exit.Stderr
				}
				t.Fatalf("exit %d want %d (%v), fixture stderr: %s", code, want, err, stderr)
			}
			var run platform.Run
			if err = json.Unmarshal(stdout, &run); err != nil {
				t.Fatalf("nonzero exit lost JSON: %v %s", err, stdout)
			}
			status := mode
			if mode == "completed" {
				status = "succeeded"
			}
			if run.Status != status {
				t.Fatal("wrong terminal status", run.Status)
			}
		})
	}
}

func TestCoverageStopExitCode(t *testing.T) {
	if got := auditExitCode(platform.Run{Status: "incomplete"}, errors.New("coverage stopped")); got != 2 {
		t.Fatalf("coverage stop exit %d", got)
	}
	if got := auditExitCode(platform.Run{Status: "failed"}, errors.New("transport failed")); got != 1 {
		t.Fatalf("failure exit %d", got)
	}
}
