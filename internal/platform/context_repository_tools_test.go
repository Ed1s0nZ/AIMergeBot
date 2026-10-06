package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func crossGitFixture(t *testing.T) (*GitRepository, Snapshot, Finding, map[int]ContextSource) {
	t.Helper()
	root, snap := localGitFixture(t)
	related, other := localGitFixture(t)
	commit := func(repo *GitRepository, text, message string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo.Directory, "guard.any"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "guard.any"}, {"commit", "-m", message}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = repo.Directory
			cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
			if _, err := cmd.CombinedOutput(); err != nil {
				t.Fatal("fixture commit", err)
			}
		}
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = repo.Directory
		raw, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(raw))
	}
	snap.BaseSHA = snap.HeadSHA
	snap.HeadSHA = commit(root, "safe\ndanger(input)\n", "primary change")
	snap.MRIID = 1
	fixed := commit(related, "safe\ndanger(input)\ncontext-only needle\n", "fixed context")
	commit(related, "latest safe replacement\n", "unselected later commit")
	other.ProjectID = 2
	other.SourceProjectID = 2
	other.BaseSHA = fixed
	other.HeadSHA = fixed
	snap.AuditPolicy = &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: fixed}}}
	f := Finding{ID: "risk", File: "guard.any", Side: "head", Line: 2, Type: "authorization", Severity: "high", Title: "Untrusted sensitive operation", Description: "Untrusted input reaches a sensitive operation without a guard", Evidence: "danger(input)", Trigger: "Attacker controls input", Suggestion: "Check access before the operation", Confidence: "candidate"}
	return root, snap, f, map[int]ContextSource{2: {Repository: related, Snapshot: other}}
}
func crossTools(root Repository, snap Snapshot, sources map[int]ContextSource) *auditTools {
	return &auditTools{repo: root, snap: snap, contextSources: sources, cache: map[string]string{}, scope: DiffScope{Added: map[string]map[int]bool{"guard.any": {2: true}}}}
}
func TestContextToolsUseFixedGitSourceAndDoNotOverwritePrimaryCache(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	ctx := context.Background()
	primary, _ := tools.file(ctx, readArgs{Path: "guard.any", Start: 1, End: 3})
	related, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any", Start: 1, End: 3})
	if related.Error != "" || related.RepositoryID != 2 || related.HeadSHA != snap.AuditPolicy.ContextRepositories[0].SHA || related.HeadSHA == snap.HeadSHA || !strings.Contains(related.Text, "context-only needle") || strings.Contains(related.Text, "latest safe") {
		t.Fatal("wrong context object", related.Error)
	}
	if primary.RepositoryID != 0 || primary.HeadSHA != snap.HeadSHA || strings.Contains(primary.Text, "context-only") {
		t.Fatal("primary/context cache identity mixed")
	}
	searched, _ := tools.contextSearch(ctx, contextSearchArgs{RepositoryID: 2, Query: "needle", Path: "guard.any"})
	if searched.Error != "" || !searched.EvidenceEligible || !strings.Contains(searched.Text, "guard.any:3:") {
		t.Fatal("fixed context search", searched.Error)
	}
	directory, _ := tools.contextDirectory(ctx, contextDirectoryArgs{RepositoryID: 2, Path: "nested", Depth: 2})
	if directory.Error != "" || directory.EvidenceEligible || !strings.Contains(directory.Text, "route.config") {
		t.Fatal("directory source", directory.Error)
	}
	listed, _ := tools.contextRepositories(ctx, struct{}{})
	if listed.EvidenceEligible || len(listed.Repositories) != 1 || !listed.Repositories[0].Available {
		t.Fatal("repository enumeration")
	}
	denied, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 999, Path: "guard.any"})
	if denied.Error == "" || denied.Text != "" || denied.EvidenceEligible {
		t.Fatal("arbitrary repository ID accepted")
	}
	if len(tools.trace) != 6 || tools.calls != 6 || len(tools.contextReaders[2].trace) != 0 {
		t.Fatal("hidden nested observations or duplicate tool accounting")
	}
}
func TestContextCacheCannotBypassRevocationAndSharedToolBudget(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	ctx := context.Background()
	var allowed atomic.Bool
	allowed.Store(true)
	source := sources[2]
	source.Authorize = func(context.Context) error {
		if !allowed.Load() {
			return ErrContextRepository
		}
		return nil
	}
	sources[2] = source
	tools := crossTools(root, snap, sources)
	first, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	allowed.Store(false)
	denied, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	if denied.Error == "" || denied.Text != "" || denied.EvidenceEligible {
		t.Fatal("revoked cached source returned")
	}
	allowed.Store(true)
	limited := crossTools(root, snap, sources)
	limited.maxCalls = 1
	limited.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	blocked, _ := limited.contextSearch(ctx, contextSearchArgs{RepositoryID: 2, Query: "needle"})
	if blocked.Error == "" || blocked.Text != "" || len(limited.trace) != 2 {
		t.Fatal("context calls bypass shared budget")
	}
}
func TestContextObservationCannotStandInForPrimaryFindingOrVerifierAnchor(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	ctx := context.Background()
	tools := crossTools(root, snap, sources)
	related, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: f.File, Start: 2, End: 2})
	item := Investigation{ID: "context", Claim: "sensitive operation reachable", Status: "supported", ObservationIDs: []string{related.ObservationID}, Evidence: []string{"static context"}}
	tools.ledger = map[string]Investigation{item.ID: item}
	f.Confidence = "supported"
	f.InvestigationID = item.ID
	f.ObservationIDs = item.ObservationIDs
	if _, err := tools.acceptFinding(ctx, f); err == nil {
		t.Fatal("related identical snippet substituted for primary anchor")
	}
	primary, _ := tools.file(ctx, readArgs{Path: f.File, Start: 2, End: 2})
	item.ObservationIDs = append(item.ObservationIDs, primary.ObservationID)
	tools.ledger[item.ID] = item
	f.ObservationIDs = item.ObservationIDs
	if _, err := tools.acceptFinding(ctx, f); err != nil {
		t.Fatal("root anchor plus context rejected", err)
	}
	fresh := crossTools(root, snap, sources)
	fresh.stage = "verification"
	fresh.observationPrefix = "verify-risk-observation"
	extra, _ := fresh.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: f.File, Start: 2, End: 2})
	verdict := verificationInput{ClaimCoverage: "full", Status: "supported", Reason: "static conditional risk", Limitations: []string{}, ObservationIDs: []string{extra.ObservationID}}
	if _, err := validateVerification(verdict, snap, f, fresh.trace); err == nil {
		t.Fatal("context observation endorsed primary without fresh read")
	}
	anchor, _ := fresh.file(ctx, readArgs{Path: f.File, Start: 2, End: 2})
	verdict.ObservationIDs = append(verdict.ObservationIDs, anchor.ObservationID)
	if _, err := validateVerification(verdict, snap, f, fresh.trace); err != nil {
		t.Fatal("verifier rejected real fixed context", err)
	}
}
func TestContextToolsAreCallableThroughRealEinoSDK(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		n := calls.Add(1)
		message := map[string]any{"role": "assistant"}
		finish := "stop"
		if n == 1 {
			seen := map[string]bool{}
			for _, tool := range request.Tools {
				seen[tool.Function.Name] = true
			}
			for _, name := range []string{"list_repositories", "read_repository_file", "search_repository_code", "list_repository_directory"} {
				if !seen[name] {
					t.Error("missing schema", name)
				}
			}
			message["tool_calls"] = []any{map[string]any{"id": "context", "type": "function", "function": map[string]string{"name": "read_repository_file", "arguments": `{"repository_id":2,"path":"guard.any","start":1,"end":3}`}}}
			finish = "tool_calls"
		} else {
			found := false
			for _, msg := range request.Messages {
				if msg.Role == "tool" {
					var out toolOutput
					if json.Unmarshal([]byte(msg.Content), &out) == nil && out.RepositoryID == 2 && out.HeadSHA == sources[2].Snapshot.HeadSHA && strings.Contains(out.Text, "context-only needle") {
						found = true
					}
				}
			}
			if !found {
				t.Error("SDK lost source provenance")
			}
			message["content"] = `{"findings":[],"summary":"synthetic context inspected, no safety certification","coverage_notes":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	e := &EinoAuditor{Repository: root, ContextSources: sources, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "context-fixture", MaxSteps: 4, MaxToolCalls: 4}}
	result, trace, err := e.Audit(context.Background(), snap, BuildDiff([]Change{{NewPath: "guard.any", Diff: "@@ -1 +1,2 @@\n safe\n+danger(input)"}}, nil, 96000))
	if err != nil || (len(result.CoverageNotes) != 2 || !hasPlanGap(result.CoverageNotes) || !slices.Contains(result.CoverageNotes, "Bounded tool output: list_files")) || calls.Load() != 2 {
		t.Fatal("SDK context audit", err, result.CoverageNotes)
	}
	count := 0
	for _, item := range trace {
		if item.Name == "read_repository_file" {
			count++
			var out toolOutput
			if json.Unmarshal([]byte(item.Output), &out) != nil || out.RepositoryID != 2 || out.HeadSHA != sources[2].Snapshot.HeadSHA {
				t.Fatal("persisted wrong provenance")
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate scoped tool trace", fmt.Sprint(count))
	}
	// Exercise the same real SDK tool through queue ownership, per-read store
	// authorization and durable Worker checkpoints, rather than only Auditor.
	s, admin, _, _ := contextFixture(t)
	ctx := context.Background()
	if err = s.SaveContextRepositories(ctx, 1, snap.AuditPolicy.ContextRepositories, admin.ID); err != nil {
		t.Fatal(err)
	}
	calls.Store(0)
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Repository: root, Auditor: e, Workers: 1, Timeout: 30 * time.Second}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, readErr := s.Run(ctx, id)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if run.Status == "pending" || run.Status == "running" {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if run.Status != "incomplete" || len(run.Result.CoverageNotes) != 2 || !hasPlanGap(run.Result.CoverageNotes) || !slices.Contains(run.Result.CoverageNotes, "Bounded tool output: list_files") || calls.Load() != 2 {
			t.Fatal("worker context audit failed", run.Status, run.Error, calls.Load(), run.Result.CoverageNotes)
		}
		observations := 0
		for _, item := range run.Trace {
			if item.Name != "read_repository_file" {
				continue
			}
			observations++
			var out toolOutput
			if json.Unmarshal([]byte(item.Output), &out) != nil || out.RepositoryID != 2 || out.HeadSHA != sources[2].Snapshot.HeadSHA || !strings.Contains(out.Text, "context-only needle") {
				t.Fatal("worker lost context source")
			}
		}
		if observations != 1 {
			t.Fatal("worker duplicated or omitted scoped trace", observations)
		}
		return
	}
	t.Fatal("worker context test did not finish")
}
