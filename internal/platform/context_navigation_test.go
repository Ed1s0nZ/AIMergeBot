package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrimaryFixedContextPreflightBeforeSDKRequest(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	for _, mode := range []string{"ready", "revoked", "checkpoint_failed", "canceled", "no_context"} {
		t.Run(mode, func(t *testing.T) {
			var modelCalls atomic.Int32
			checkpointed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelCalls.Add(1)
				var request struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				found := strings.Contains(request.Messages[1].Content, "Configured fixed context navigation")
				if mode == "ready" && (!found || !checkpointed || !strings.Contains(request.Messages[1].Content, `"available":true`) || !strings.Contains(request.Messages[1].Content, `"evidence_eligible":false`)) {
					t.Error("navigation or durable preflight missing")
				}
				if mode == "no_context" && found {
					t.Error("no-context audit changed")
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": `{"findings":[],"summary":"synthetic summary","coverage_notes":[]}`}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
			}))
			defer server.Close()
			copySources := map[int]ContextSource{2: sources[2]}
			if mode == "revoked" {
				s := copySources[2]
				s.Authorize = func(context.Context) error { return ErrContextRepository }
				copySources[2] = s
			}
			current := snap
			if mode == "no_context" {
				current.AuditPolicy = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			a := &EinoAuditor{Repository: root, ContextSources: copySources, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 4, PrimaryOnly: true, Progress: func(_ AuditResult, trace []ToolTrace) error {
				if len(trace) > 0 && trace[len(trace)-1].Name == "list_repositories" {
					checkpointed = true
				}
				if mode == "checkpoint_failed" {
					return fmt.Errorf("fixture checkpoint failure")
				}
				return nil
			}}}
			result, _, err := a.Audit(ctx, current, DiffScope{})
			if mode == "ready" || mode == "no_context" {
				if err != nil || modelCalls.Load() != 1 {
					t.Fatal(result, err, modelCalls.Load())
				}
				if mode == "ready" && !strings.Contains(strings.Join(result.CoverageNotes, " "), "did not inspect configured fixed context repository 2") {
					t.Fatal("unread source omitted", result)
				}
			} else if err == nil || modelCalls.Load() != 0 {
				t.Fatal("failed preflight sent model request", mode, err, modelCalls.Load())
			}
		})
	}
}

func TestPrimaryContextInspectionNotInferredFromLaterStageOrEnumeration(t *testing.T) {
	snap := Snapshot{AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "fixed"}}}}
	raw, _ := json.Marshal(toolOutput{RepositoryID: 2, BaseSHA: "fixed", HeadSHA: "fixed", Text: "source", EvidenceEligible: true})
	good := ToolTrace{Name: "read_repository_file", Output: string(raw)}
	if len(primaryContextCoverage(snap, []ToolTrace{good})) != 0 {
		t.Fatal("fresh source ignored")
	}
	for _, mode := range []string{"verification", "failed", "directory", "old_sha", "not_eligible"} {
		bad := good
		switch mode {
		case "verification":
			bad.Stage = "verification"
		case "failed":
			bad.Error = "revoked"
		case "directory":
			bad.Name = "list_repository_directory"
		case "old_sha":
			bad.Output = `{"repository_id":2,"base_sha":"old","head_sha":"old","text":"source","evidence_eligible":true}`
		case "not_eligible":
			bad.Output = `{"repository_id":2,"base_sha":"fixed","head_sha":"fixed","text":"source"}`
		}
		if len(primaryContextCoverage(snap, []ToolTrace{bad})) != 1 {
			t.Fatal("invalid primary coverage accepted", mode)
		}
	}
}

func TestContextEnumerationRechecksEarlierAuthorization(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	var calls atomic.Int32
	s := sources[2]
	s.Authorize = func(context.Context) error {
		if calls.Add(1) > 1 {
			return ErrContextRepository
		}
		return nil
	}
	sources[2] = s
	out, _ := crossTools(root, snap, sources).contextRepositories(context.Background(), struct{}{})
	if out.Error == "" || len(out.Repositories) != 0 {
		t.Fatal("stale aggregate exposed", out)
	}
}
