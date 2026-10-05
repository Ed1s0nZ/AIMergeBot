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
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func assertChunkAnchors(t *testing.T, c Change, limit int) {
	t.Helper()
	chunks, notes := splitChangeDiff(c, limit)
	if len(chunks) < 2 || len(notes) != 0 {
		t.Fatalf("chunks=%d gaps=%v", len(chunks), notes)
	}
	original := BuildDiff([]Change{c}, nil, 1<<24)
	merged := DiffScope{Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Metadata: map[string]GitChangeMetadata{}}
	for _, chunk := range chunks {
		scope := BuildDiff([]Change{chunk}, nil, limit)
		if len(scope.Text) == 0 || len(scope.Text) > limit {
			t.Fatal("unbounded or empty chunk")
		}
		mergeScopeAnchors(&merged, scope)
	}
	if !reflect.DeepEqual(original.Added, merged.Added) || !reflect.DeepEqual(original.Removed, merged.Removed) {
		t.Fatal("split changed original line anchors")
	}
	// Several chunks can share a group. BuildDiff must also union their anchors.
	combined := BuildDiff(chunks, nil, 1<<24)
	if !reflect.DeepEqual(original.Added, combined.Added) || !reflect.DeepEqual(original.Removed, combined.Removed) {
		t.Fatal("same-file chunk anchors overwritten")
	}
}

func TestDiffChunksPreserveLineCoordinatesWithoutLanguageParser(t *testing.T) {
	for _, mode := range []string{"mixed", "added", "deleted", "renamed", "multiple-hunks"} {
		t.Run(mode, func(t *testing.T) {
			c := Change{NewPath: "文件.unknown", OldPath: "文件.unknown"}
			body := ""
			for i := 0; i < 200; i++ {
				body += fmt.Sprintf("-旧内容%d\n+新内容%d\n", i, i)
			}
			c.Diff = "@@ -10,200 +20,200 @@ description\n" + body
			switch mode {
			case "added":
				c.Added = true
				c.Diff = "@@ -0,0 +1,200 @@\n" + strings.Repeat("+新增\n", 200)
			case "deleted":
				c.Deleted = true
				c.Diff = "@@ -1,200 +0,0 @@\n" + strings.Repeat("-删除\n", 200)
			case "renamed":
				c.Renamed = true
				c.OldPath = "old.unknown"
			case "multiple-hunks":
				c.Diff += "@@ -500,200 +700,200 @@\n" + body
			}
			c.Diff += "\\ No newline at end of file\n"
			assertChunkAnchors(t, c, 1024)
		})
	}
}

func TestRealGitLargeDiffUsesBoundedChunksWithCompleteAnchors(t *testing.T) {
	repo, snap := localGitFixture(t)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo.Directory
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git: %v", err)
		}
		return strings.TrimSpace(string(raw))
	}
	file := filepath.Join(repo.Directory, "large.unknown")
	if err := os.WriteFile(file, []byte(strings.Repeat("old "+strings.Repeat("a", 35)+"\n", 2000)), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "large.unknown")
	git("commit", "-m", "large base")
	snap.BaseSHA = git("rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte(strings.Repeat("new "+strings.Repeat("b", 35)+"\n", 2000)), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "large.unknown")
	git("commit", "-m", "large head")
	snap.HeadSHA = git("rev-parse", "HEAD")
	changes, notes, err := repo.Changes(context.Background(), snap)
	if err != nil || len(changes) != 1 || len(notes) != 0 {
		t.Fatal("real Git changes", err)
	}
	if len(BuildDiff(changes, nil, 96*1024).Text) != 0 {
		t.Fatal("fixture must exceed old single-file limit")
	}
	assertChunkAnchors(t, changes[0], auditGroupBytes)
	plan := PlanAuditGroups(changes, nil)
	if len(plan.Groups) < 2 || len(plan.Notes) != 0 {
		t.Fatal("large real file still omitted", len(plan.Groups), plan.Notes)
	}
	original := BuildDiff(changes, nil, 1<<24)
	merged := DiffScope{Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Metadata: map[string]GitChangeMetadata{}}
	for _, group := range plan.Groups {
		mergeScopeAnchors(&merged, group.Scope)
	}
	if !reflect.DeepEqual(original.Added, merged.Added) || !reflect.DeepEqual(original.Removed, merged.Removed) {
		t.Fatal("planner lost real Git anchors")
	}
}

func TestDiffChunksReportOversizedLinesMalformedInputAndTotalCap(t *testing.T) {
	huge := strings.Repeat("x", auditGroupBytes+100)
	c := Change{NewPath: "huge.unknown", Diff: "@@ -0,0 +1,3 @@\n+first\n+" + huge + "\n+last\n"}
	chunks, notes := splitChangeDiff(c, auditGroupBytes)
	if len(chunks) != 2 || len(notes) != 1 {
		t.Fatal("oversized source line silently lost", len(chunks), notes)
	}
	after := BuildDiff(chunks, nil, 1<<24)
	if !after.Added[c.NewPath][1] || after.Added[c.NewPath][2] || !after.Added[c.NewPath][3] {
		t.Fatal("omitted line shifted remaining anchors")
	}
	for _, header := range []string{"@@ -0,0 +1,999 @@", "@@ -999999999999999999999999,1 +1,1 @@"} {
		bad := Change{NewPath: "bad.unknown", Diff: header + "\n+" + huge + "\n"}
		units, gaps := splitChangeDiff(bad, auditGroupBytes)
		if len(units) != 0 || len(gaps) == 0 {
			t.Fatal("malformed hunk created anchors")
		}
	}
	big := Change{NewPath: "over.unknown", Diff: "@@ -0,0 +1,20000 @@\n" + strings.Repeat("+some-language-free-content\n", 20000)}
	plan := PlanAuditGroups([]Change{big}, nil)
	total := 0
	for _, group := range plan.Groups {
		total += len(group.Scope.Text)
	}
	if len(plan.Groups) != auditMaxGroups || total > auditTotalDiffBytes || len(plan.Notes) == 0 {
		t.Fatal("unbounded or silent partial plan", len(plan.Groups), total)
	}
}

func TestChunkScopeDoesNotHideEarlierGroupFailure(t *testing.T) {
	c := Change{NewPath: "large.unknown", Diff: "@@ -0,0 +1,2000 @@\n" + strings.Repeat("+some-language-free-content\n", 2000)}
	run := Run{Snapshot: Snapshot{AuditPolicy: &AuditPolicy{}}}
	run.Result.AuditGroups = []AuditGroupProgress{{Files: []string{c.NewPath}, Status: "failed"}, {Files: []string{c.NewPath}, Status: "completed"}}
	scope := makeRunScope(run, []Change{c}, nil)
	if len(scope.Files) != 1 || scope.Files[0].Status != "group_failed" {
		t.Fatal("last successful chunk hid earlier failure")
	}
}

func TestLargeFileWorkerReachesGroupedModelInsteadOfEmptyScopeReturn(t *testing.T) {
	s, runner, parent := followupFixture(t)
	ctx := context.Background()
	c := Change{NewPath: "large.unknown", Diff: "@@ -0,0 +1,5000 @@\n" + strings.Repeat("+some-language-free-content\n", 5000)}
	if len(BuildDiff([]Change{c}, nil, 96*1024).Text) != 0 {
		t.Fatal("fixture not oversized")
	}
	repo := followupRepo{expected: parent.Snapshot, changes: []Change{c}}
	runner.Repository = repo
	var primaryCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		result := map[string]any{"summary": "synthetic empty findings, no safety claim", "coverage_notes": []string{}}
		if len(input.Messages) > 0 && !strings.HasPrefix(input.Messages[0].Content, "Summarize a grouped") {
			result["findings"] = []Finding{}
			primaryCalls.Add(1)
		}
		content, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	runner.Auditor = &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "chunk-fixture", MaxSteps: 4, MaxToolCalls: 80}}
	runner.Workers = 1
	runner.Timeout = 60 * time.Second
	id, _, err := runner.SubmitFollowup(ctx, parent.ID, 1, []string{c.NewPath})
	if err != nil {
		t.Fatal(err)
	}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, err := s.Run(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != "pending" && run.Status != "running" {
			if len(run.Result.AuditGroups) < 2 || primaryCalls.Load() != int32(len(run.Result.AuditGroups)) {
				t.Fatal("worker bypassed chunk plan", run.Status, primaryCalls.Load(), len(run.Result.AuditGroups))
			}
			for _, group := range run.Result.AuditGroups {
				if group.Status != "completed" {
					t.Fatal("chunk failed", group.ID, group.Status)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker did not finish bounded chunk test")
}

func TestChunkScopeReportsPartialInputBeforeAnyGroupFailure(t *testing.T) {
	c := Change{NewPath: "over.unknown", Diff: "@@ -0,0 +1,20000 @@\n" + strings.Repeat("+some-language-free-content\n", 20000)}
	run := Run{Snapshot: Snapshot{AuditPolicy: &AuditPolicy{}}}
	scope := makeRunScope(run, []Change{c}, nil)
	if len(scope.Files) != 1 || scope.Files[0].Status != "partial_input" || len(scope.Notes) == 0 {
		t.Fatal("partial file labeled fully included")
	}
}
