package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"
)

type unavailableFixtureRepo struct {
	runRepo
	failure error
}

func (r unavailableFixtureRepo) ReadFile(context.Context, Snapshot, string, bool) (string, error) {
	return "", r.failure
}

func TestGitUnavailableClassificationDoesNotBlockRepairableErrors(t *testing.T) {
	if !unavailableGitExecution(fmt.Errorf("wrapped: %w", exec.ErrNotFound)) {
		t.Fatal("missing executable not classified")
	}
	for _, code := range []int{1, 69, 128} {
		err := exec.Command("/bin/sh", "-c", fmt.Sprintf("exit %d", code)).Run()
		if unavailableGitExecution(err) != (code == 69) {
			t.Fatal("wrong classification", code, err)
		}
	}
	if unavailableGitExecution(context.Canceled) || unavailableGitExecution(errors.New("Git file not found")) {
		t.Fatal("ordinary error classified as infrastructure loss")
	}
}
func TestRepositoryUnavailableStopsSDKAndGroupedModelRequests(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		for _, fatal := range []bool{false, true} {
			t.Run(fmt.Sprintf("grouped_%t_fatal_%t", grouped, fatal), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					message := map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"fixture result","coverage_notes":[]}`}
					finish := "stop"
					if n == 1 {
						finish = "tool_calls"
						message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"service.any","start":1,"end":1}`}}}}
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}, "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
				}))
				defer server.Close()
				failure := errors.New("requested file not found")
				if fatal {
					failure = fmt.Errorf("reader stopped: %w", ErrRepositoryUnavailable)
				}
				auditor := &EinoAuditor{Repository: unavailableFixtureRepo{failure: failure}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 4, MaxToolCalls: 8}}
				snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
				scope := DiffScope{Added: map[string]map[int]bool{"service.any": {1: true}}}
				var result AuditResult
				var trace []ToolTrace
				var err error
				if grouped {
					result, trace, err = auditor.AuditGroups(context.Background(), snap, AuditPlan{Groups: []AuditGroup{{ID: "first", Files: []string{"service.any"}, Scope: scope}, {ID: "second", Files: []string{"other.any"}, Scope: scope}}})
				} else {
					result, trace, err = auditor.Audit(context.Background(), snap, scope)
				}
				if fatal {
					if !errors.Is(err, ErrRepositoryUnavailable) || calls.Load() != 1 || len(result.CoverageNotes) == 0 {
						t.Fatal("billed after unavailable reader", calls.Load(), err, result.CoverageNotes)
					}
					if grouped && (result.AuditGroups[0].Status != "failed" || result.AuditGroups[1].Status != "unprocessed") {
						t.Fatal("lost group coverage", result.AuditGroups)
					}
				} else if err != nil || calls.Load() < 2 {
					t.Fatal("ordinary source failure blocked repair", calls.Load(), err)
				}
				found := false
				for _, tr := range trace {
					if tr.Name == "read_file" && tr.Error != "" {
						found = true
					}
				}
				if !found {
					t.Fatal("source failure trace lost")
				}
			})
		}
	}
}

type partiallyUnavailableRepo struct{ runRepo }

func (partiallyUnavailableRepo) ReadFile(_ context.Context, _ Snapshot, path string, _ bool) (string, error) {
	if path == "blocked.any" {
		return "", ErrRepositoryUnavailable
	}
	return "exec(request.input)\n", nil
}
func TestRepositoryUnavailableRetainsAcceptedSDKCandidate(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			name := "read_file"
			var args any = readArgs{Path: "service.any", Start: 1, End: 1}
			if n == 2 {
				name = "submit_finding"
				args = Finding{File: "service.any", Side: "head", Line: 1, Severity: "high", Type: "Command injection", Title: "Untrusted input reaches a command", Description: "Under an attacker-controlled request assumption, the changed operation permits command execution.", Evidence: "exec(request.input)", Trigger: "Caller passes attacker-controlled request input", Suggestion: "Use a bounded argument API", Confidence: "candidate"}
			}
			if n == 3 {
				args = readArgs{Path: "blocked.any", Start: 1, End: 1}
			}
			raw, _ := json.Marshal(args)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}, "choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": fmt.Sprintf("step%d", n), "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}}}}}}})
		}))
		auditor := &EinoAuditor{Repository: partiallyUnavailableRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 6, MaxToolCalls: 8}}
		snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
		scope := DiffScope{Added: map[string]map[int]bool{"service.any": {1: true}}}
		var result AuditResult
		var err error
		if grouped {
			result, _, err = auditor.AuditGroups(context.Background(), snap, AuditPlan{Groups: []AuditGroup{{ID: "first", Files: []string{"service.any"}, Scope: scope}}})
		} else {
			result, _, err = auditor.Audit(context.Background(), snap, scope)
		}
		server.Close()
		if !errors.Is(err, ErrRepositoryUnavailable) || calls.Load() != 3 || len(result.Findings) != 1 || result.Findings[0].Confidence != "candidate" || result.Findings[0].Evidence != "exec(request.input)" {
			t.Fatal("accepted candidate lost or more models billed", grouped, calls.Load(), err, result.Findings)
		}
	}
}
