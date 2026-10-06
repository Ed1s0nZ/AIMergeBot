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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func metadataGitFixture(t *testing.T) (*GitRepository, Snapshot, []Change) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		raw, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture: %v %s", err, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.com")
	for p, contents := range map[string][]byte{"task.any": []byte("untrusted(command)\n"), "rename.any": []byte("unchanged\n"), "link.any": []byte("target\n"), "binary.dat": {0, 1}} {
		if err := os.WriteFile(filepath.Join(dir, p), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-m", "initial")
	initial := git("rev-parse", "HEAD")
	git("update-index", "--add", "--cacheinfo", "160000,"+initial+",module")
	git("commit", "-m", "base with gitlink")
	base := git("rev-parse", "HEAD")
	if err := os.Chmod(filepath.Join(dir, "task.any"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "rename.any"), filepath.Join(dir, "-renamed\n空 格.any")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "link.any")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../outside-must-not-be-read", filepath.Join(dir, "link.any")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.dat"), []byte{0, 2}, 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("c", 40)+",module")
	git("commit", "-m", "metadata changes")
	head := git("rev-parse", "HEAD")
	repo := &GitRepository{Directory: dir}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 7, BaseSHA: base, HeadSHA: head}
	changes, _, err := repo.Changes(context.Background(), snap)
	if err != nil {
		t.Fatal(err)
	}
	return repo, snap, changes
}

func TestRealGitMetadataPreservesModesRenamesSymlinksAndGitlinks(t *testing.T) {
	repo, snap, changes := metadataGitFixture(t)
	scope := BuildDiff(changes, nil, 96*1024)
	if len(scope.Metadata) != 5 || len(scope.Notes) != 1 || !strings.Contains(scope.Notes[0], "binary.dat") {
		t.Fatal(scope.Metadata, scope.Notes)
	}
	chmod := scope.Metadata["task.any"]
	if chmod.Base.Mode != "100644" || chmod.Head.Mode != "100755" || chmod.Base.ObjectID != chmod.Head.ObjectID || len(scope.Added["task.any"]) != 0 {
		t.Fatal(chmod)
	}
	rename := scope.Metadata["-renamed\n空 格.any"]
	if rename.Kind != "rename" || rename.OldPath != "rename.any" || !rename.metadataOnly() {
		t.Fatal(rename)
	}
	link := scope.Metadata["link.any"]
	if link.Kind != "type_change" || link.Head.Mode != "120000" {
		t.Fatal(link)
	}
	contents, err := repo.ReadFile(context.Background(), snap, "link.any", false)
	if err != nil || contents != "../../../outside-must-not-be-read" {
		t.Fatal("symlink dereferenced instead of reading blob", contents, err)
	}
	module := scope.Metadata["module"]
	if !module.metadataOnly() || module.Head.Mode != "160000" || module.Head.ObjectID != strings.Repeat("c", 40) || len(scope.Added["module"]) != 0 {
		t.Fatal(module)
	}
	excluded := BuildDiff(changes, []string{"any"}, 96*1024)
	if len(excluded.Excluded) != 3 || len(excluded.Metadata) != 2 {
		t.Fatal(excluded)
	}
	bounded := BuildDiff(changes, nil, 1)
	if bounded.Text != "" || len(bounded.Metadata) != 0 || len(bounded.Notes) == 0 {
		t.Fatal("omitted metadata accepted as an anchor", bounded)
	}
}

func TestRawGitMetadataRejectsTruncationModesObjectsAndUnchangedEntries(t *testing.T) {
	object := strings.Repeat("a", 40)
	for _, raw := range []string{":100644 100755 " + object + " " + object + " M\x00file", ":bad 100755 " + object + " " + object + " M\x00file\x00", ":100644 100755 short " + object + " M\x00file\x00", ":100644 100644 " + object + " " + object + " M\x00file\x00", ":100644 100644 " + object + " " + object + " R100\x00old\x00"} {
		if _, err := parseRawGitChanges(raw); err == nil {
			t.Fatal("malformed raw record accepted", raw)
		}
	}
	raw := ":000000 100644 " + strings.Repeat("0", 64) + " " + strings.Repeat("b", 64) + " A\x00file\x00"
	changes, err := parseRawGitChanges(raw)
	if err != nil || len(changes) != 1 || changes[0].Metadata.Base != nil || changes[0].Metadata.Kind != "add" {
		t.Fatal(changes, err)
	}
}

func TestExcludedFileMetadataGapsDoNotTurnSkippedAuditIncomplete(t *testing.T) {
	changes := []Change{{OldPath: "README.md", NewPath: "README.md", Added: true, Notes: []string{"Git metadata unavailable or lookup budget exceeded: README.md", "GitLab omitted diff content: README.md"}}}
	scope := BuildDiff(changes, []string{"md"}, 96*1024)
	if scope.Text != "" || len(scope.Notes) != 0 || len(scope.Metadata) != 0 || len(scope.Excluded) != 1 {
		t.Fatal(scope)
	}
	s := testStore(t)
	ctx := context.Background()
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "exclusion", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	r := &Runner{Store: s, Repository: metadataRunRepo{&GitRepository{}, snap, changes}, Excluded: []string{"md"}, Workers: 1, Timeout: time.Minute}
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	id, _, err := r.Submit(ctx, 1, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, id, "skipped")
}

func metadataFinding(metadata GitChangeMetadata) Finding {
	return Finding{AnchorType: "git_metadata", Side: "head", File: metadata.NewPath, Line: 0, Severity: "medium", Type: "execution policy", Title: "执行模式需要复核", Description: "此项是合成风险候选，不代表可利用性已验证。", Evidence: metadata.canonical(), Trigger: "部署环境按可执行位启动此文件且未做额外校验时", Suggestion: "确认执行策略和调用入口", Confidence: "candidate"}
}
func TestMetadataFindingValidationAndObservationProvenance(t *testing.T) {
	repo, snap, changes := metadataGitFixture(t)
	scope := BuildDiff(changes, nil, 96*1024)
	ctx := context.Background()
	f := metadataFinding(scope.Metadata["task.any"])
	result := AuditResult{Findings: []Finding{f}}
	if err := ValidateFindings(ctx, repo, snap, scope, &result); err != nil || result.Findings[0].Metadata == nil {
		t.Fatal(result, err)
	}
	for _, mutation := range []func(*Finding){func(f *Finding) { f.Line = 1 }, func(f *Finding) { f.Evidence = "old mode 100644" }, func(f *Finding) { f.File = "unchanged.any" }, func(f *Finding) { f.AnchorType = "line" }, func(f *Finding) { copy := scope.Metadata["task.any"]; copy.Kind = "delete"; f.Metadata = &copy }} {
		bad := f
		mutation(&bad)
		result.Findings = []Finding{bad}
		if err := ValidateFindings(ctx, repo, snap, scope, &result); err == nil {
			t.Fatal("tampered metadata anchor accepted", bad)
		}
	}
	tools := toolsFor(repo, snap)
	tools.scope = scope
	observation, _ := tools.changeMetadata(ctx, metadataArgs{Path: "task.any"})
	if observation.Error != "" || observation.Metadata == nil || observation.Text != f.Evidence {
		t.Fatal(observation)
	}
	_, _ = tools.record(ctx, Investigation{ID: "mode", Claim: "execution mode changed"})
	updated, _ := tools.update(ctx, Investigation{ID: "mode", Claim: "execution mode changed", Status: "supported", Evidence: []string{f.Evidence}, ObservationIDs: []string{observation.ObservationID}})
	if updated.Error != "" {
		t.Fatal(updated)
	}
	f.Confidence = "supported"
	f.InvestigationID = "mode"
	f.ObservationIDs = []string{observation.ObservationID}
	if _, err := tools.acceptFinding(ctx, f); err != nil {
		t.Fatal(err)
	}
	forged, _ := tools.invoke("read_file", readArgs{Path: "task.any"}, func() (toolOutput, error) { return toolOutput{Text: f.Evidence}, nil })
	updateForged, _ := tools.update(ctx, Investigation{ID: "mode", Claim: "execution mode changed", Status: "supported", Evidence: []string{f.Evidence}, ObservationIDs: []string{observation.ObservationID, forged.ObservationID}})
	if updateForged.Error != "" {
		t.Fatal(updateForged)
	}
	f.ObservationIDs = []string{forged.ObservationID}
	if _, err := tools.acceptFinding(ctx, f); err == nil {
		t.Fatal("source text containing metadata JSON counted as metadata provenance")
	}
	f.ObservationIDs = []string{"observation-2"} // Process ledger output cannot establish metadata provenance.
	if _, err := tools.acceptFinding(ctx, f); err == nil {
		t.Fatal("ledger counted as source metadata")
	}
	missing, _ := tools.changeMetadata(ctx, metadataArgs{Path: "unchanged.any"})
	if missing.Error == "" {
		t.Fatal(missing)
	}
}

func TestGitLabMetadataReadsPinnedForkSidesAndContinuesPages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		base := r.URL.Path == "/api/v4/projects/1/repository/tree"
		if base && (q.Get("ref") != "target-base" || q.Get("path") != "old") || !base && (r.URL.Path != "/api/v4/projects/2/repository/tree" || q.Get("ref") != "fork-head" || q.Get("path") != "new") {
			t.Errorf("unpinned request %s", r.URL.String())
		}
		if q.Get("page") == "1" {
			nodes := []map[string]string{}
			for i := 0; i < 100; i++ {
				nodes = append(nodes, map[string]string{"path": fmt.Sprintf("filler-%d", i)})
			}
			w.Header().Set("X-Next-Page", "2")
			json.NewEncoder(w).Encode(nodes)
			return
		}
		p, mode := "new/file.any", "100755"
		if base {
			p, mode = "old/file.any", "100644"
		}
		json.NewEncoder(w).Encode([]map[string]string{{"path": p, "mode": mode, "type": "blob", "id": strings.Repeat("a", 40)}})
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("synthetic", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	changes := []Change{{OldPath: "old/file.any", NewPath: "new/file.any", Renamed: true}}
	notes, err := repo.enrichMetadata(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 2, BaseSHA: "target-base", HeadSHA: "fork-head"}, changes)
	if err != nil || len(notes) != 0 || calls.Load() != 4 || changes[0].Metadata == nil || changes[0].Metadata.Head.Mode != "100755" {
		t.Fatal(changes, notes, err, calls.Load())
	}
}

func TestGitLabMetadataBudgetAndUnknownEntriesRemainCoverageGaps(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		p := r.URL.Query().Get("path") + "/file.any"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{{"path": p, "mode": "100644", "type": "blob", "id": strings.Repeat("a", 40)}})
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("synthetic", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	changes := []Change{}
	for i := 0; i < 45; i++ {
		p := fmt.Sprintf("dir-%02d/file.any", i)
		changes = append(changes, Change{OldPath: p, NewPath: p, Added: true})
	}
	notes, err := repo.enrichMetadata(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 2, BaseSHA: "base", HeadSHA: "head"}, changes)
	if err != nil || calls.Load() != 40 || len(notes) != 5 {
		t.Fatal(calls.Load(), notes, err)
	}
	for i := 40; i < 45; i++ {
		if changes[i].Metadata != nil {
			t.Fatal("budget omission fabricated an entry")
		}
	}
}

func TestGitLabMetadataFailuresDoNotInventEntries(t *testing.T) {
	for _, status := range []int{200, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, `[{"path":"file.any","mode":"160000","type":"blob","id":"invented"}]`)
			}))
			defer server.Close()
			repo, err := NewGitLabRepository("synthetic", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			changes := []Change{{OldPath: "file.any", NewPath: "file.any", Added: true}}
			notes, err := repo.enrichMetadata(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 1, HeadSHA: "head"}, changes)
			if status == 503 {
				if !retryableError(err) {
					t.Fatal("temporary upstream error lost", err)
				}
				return
			}
			if err != nil || len(notes) != 1 || changes[0].Metadata != nil {
				t.Fatal(changes, notes, err)
			}
		})
	}
}

type metadataRunRepo struct {
	*GitRepository
	snapshot Snapshot
	changes  []Change
}

func (r metadataRunRepo) Snapshot(context.Context, int, int) (Snapshot, error) {
	return r.snapshot, nil
}
func (r metadataRunRepo) Changes(context.Context, Snapshot) ([]Change, []string, error) {
	return r.changes, nil, nil
}

func TestMetadataOnlyAuditUsesEinoAndRunnerPersistsCanonicalEvidence(t *testing.T) {
	repo, snap, changes := metadataGitFixture(t)
	filtered := []Change{}
	for _, c := range changes {
		if c.NewPath == "task.any" {
			filtered = append(filtered, c)
		}
	}
	scope := BuildDiff(filtered, nil, 96*1024)
	f := metadataFinding(scope.Metadata["task.any"])
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := calls.Add(1)
		message := map[string]any{"role": "assistant"}
		finish := "stop"
		if n%3 == 1 { // Each audit reads metadata, returns a draft, then a recording final.
			finish = "tool_calls"
			message["tool_calls"] = []any{map[string]any{"id": "metadata", "type": "function", "function": map[string]string{"name": "get_change_metadata", "arguments": `{"path":"task.any"}`}}}
		} else {
			raw, _ := json.Marshal(AuditResult{Findings: []Finding{f}, Summary: "合成元数据候选，需要复核。", CoverageNotes: []string{}})
			message["content"] = string(raw)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
	}))
	defer server.Close()
	auditor := &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "metadata-fixture", MaxSteps: 5}}
	result, trace, err := auditor.Audit(context.Background(), snap, scope)
	if err != nil || len(result.Findings) != 1 || result.Findings[0].Metadata == nil || len(result.MetadataChanges) != 1 || (len(result.CoverageNotes) != 1 || !hasPlanGap(result.CoverageNotes)) || len(trace) < 2 {
		t.Fatal(result, trace, err)
	}
	store := testStore(t)
	ctx := context.Background()
	if err = store.SaveProject(ctx, Project{ID: 1, Name: "metadata", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: store, Repository: metadataRunRepo{repo, snap, filtered}, Auditor: auditor, Timeout: time.Minute, Workers: 1}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	id, _, err := runner.Submit(ctx, 1, 7, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, store, id, "incomplete")
	run, err := store.Run(ctx, id)
	if err != nil || len(run.Result.Findings) != 1 || run.Result.Findings[0].Line != 0 || run.Result.Findings[0].Metadata.Head.Mode != "100755" || len(run.Result.MetadataChanges) != 1 || calls.Load() != 6 {
		t.Fatal(run, err, calls.Load())
	}
}

func TestSequenceMetadataCitationsArePinnedNoteOnly(t *testing.T) {
	repo, snap, changes := metadataGitFixture(t)
	scope := BuildDiff(changes, nil, 96*1024)
	f := metadataFinding(scope.Metadata["task.any"])
	result := AuditResult{Findings: []Finding{f}}
	if err := ValidateFindings(context.Background(), repo, snap, scope, &result); err != nil {
		t.Fatal(err)
	}
	f = result.Findings[0]
	input := SequenceInput{Participants: []SequenceParticipant{{ID: "deploy", Label: "部署环境"}, {ID: "file", Label: "文件"}}, Steps: []SequenceStep{{From: "deploy", To: "file", Label: "执行位变化；执行条件仍需核验", Kind: "note", Certainty: "cited", Risk: true, Evidence: []SequenceReference{{AnchorType: "git_metadata", Side: "head", File: f.File, Line: 0, Snippet: f.Evidence}}}}, Limitations: []string{"未执行仓库代码"}}
	diagram, err := ValidateSequence(context.Background(), repo, snap, f, input)
	if err != nil || diagram.Status != "partial" || diagram.Steps[0].Evidence[0].Metadata == nil || diagram.Steps[0].Evidence[0].SHA != snap.HeadSHA {
		t.Fatal(diagram, err)
	}
	for _, mutation := range []func(*SequenceStep){func(s *SequenceStep) { s.Kind = "call" }, func(s *SequenceStep) { s.Evidence[0].Line = 1 }, func(s *SequenceStep) { s.Evidence[0].Snippet = "invented" }, func(s *SequenceStep) { s.Evidence[0].SHA = "wrong" }} {
		copy := input
		copy.Steps = append([]SequenceStep{}, input.Steps...)
		copy.Steps[0].Evidence = append([]SequenceReference{}, input.Steps[0].Evidence...)
		mutation(&copy.Steps[0])
		if _, err := ValidateSequence(context.Background(), repo, snap, f, copy); err == nil {
			t.Fatal("invalid metadata diagram accepted", copy)
		}
	}
}
