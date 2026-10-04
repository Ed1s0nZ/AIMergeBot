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
	"testing"
)

func localGitFixture(t *testing.T) (*GitRepository, Snapshot) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		raw, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("git fixture: %v %s", e, raw)
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "--initial-branch=main")
	git("config", "uploadpack.allowReachableSHA1InWant", "true")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.com")
	if e := os.WriteFile(filepath.Join(dir, "guard.any"), []byte("authorize(user)\nsafe\n"), 0600); e != nil {
		t.Fatal(e)
	}
	git("add", ".")
	git("commit", "-m", "initial guard")
	base := git("rev-parse", "HEAD")
	for i := 0; i < 125; i++ {
		p := filepath.Join(dir, fmt.Sprintf("file-%03d.unknown", i))
		if e := os.WriteFile(p, []byte(fmt.Sprintf("header\nneedle-%03d CALL(value)\n", i)), 0600); e != nil {
			t.Fatal(e)
		}
	}
	os.Mkdir(filepath.Join(dir, "nested"), 0700)
	os.WriteFile(filepath.Join(dir, "nested", "route.config"), []byte("auth=false\n"), 0600)
	os.WriteFile(filepath.Join(dir, "guard.any"), []byte("safe\n"), 0600)
	os.WriteFile(filepath.Join(dir, "binary.dat"), []byte{0, 1, 2}, 0600)
	os.WriteFile(filepath.Join(dir, "-strange : 路径.txt"), []byte("unicode needle\n"), 0600)
	git("add", ".")
	git("commit", "-m", "change guard and introduce calls")
	head := git("rev-parse", "HEAD")
	return &GitRepository{Directory: dir}, Snapshot{ProjectID: 1, SourceProjectID: 1, BaseSHA: base, HeadSHA: head}
}
func toolsFor(g Repository, s Snapshot) *auditTools {
	return &auditTools{repo: g, snap: s, cache: map[string]string{}}
}
func TestGitToolsSearchAllFilesAndFixedSnapshots(t *testing.T) {
	g, s := localGitFixture(t)
	ctx := context.Background()
	tools := toolsFor(g, s)
	search, e := tools.search(ctx, searchArgs{Query: "needle-124"})
	if e != nil || search.Error != "" || !strings.Contains(search.Text, "file-124.unknown:2") {
		t.Fatalf("last file omitted: %+v %v", search, e)
	}
	search, _ = tools.search(ctx, searchArgs{Query: "NEEDLE-[0-9]+", Regex: true, IgnoreCase: true, Extension: "unknown"})
	if strings.Count(search.Text, "CALL(value)") != 125 {
		t.Fatalf("regex search incomplete: %s", search.Text)
	}
	search, _ = tools.search(ctx, searchArgs{Query: "authorize", Base: true})
	if !strings.Contains(search.Text, "authorize(user)") {
		t.Fatal(search)
	}
	search, _ = tools.search(ctx, searchArgs{Query: "authorize"})
	if search.Text != "" || search.Error != "" {
		t.Fatal(search)
	}
	read, _ := tools.file(ctx, readArgs{Path: "-strange : 路径.txt", Start: 1, End: 1})
	if !strings.Contains(read.Text, "unicode needle") || read.More {
		t.Fatal(read)
	}
	binary, _ := tools.file(ctx, readArgs{Path: "binary.dat"})
	if binary.Error == "" {
		t.Fatal("binary accepted")
	}
	tree, _ := tools.directory(ctx, directoryArgs{Path: "nested", Depth: 1})
	if tree.Text != "route.config" {
		t.Fatal(tree)
	}
	changes, _, e := g.Changes(ctx, s)
	if e != nil {
		t.Fatal(e)
	}
	scope := BuildDiff(changes, nil, 96*1024)
	if !scope.Removed["guard.any"][1] {
		t.Fatal("removed guard not mapped")
	}
	tools.scope = scope
	finding := Finding{Side: "base", File: "guard.any", Line: 1, Severity: "high", Type: "authorization", Title: "guard removed", Description: "potential unauthorized access", Evidence: "authorize(user)", Trigger: "untrusted caller reaches operation", Suggestion: "restore guard", Confidence: "candidate"}
	submitted, _ := tools.submit(ctx, finding)
	if submitted.Error != "" {
		t.Fatal(submitted)
	}
	finding.Evidence = "invented"
	submitted, _ = tools.submit(ctx, finding)
	if submitted.Error == "" {
		t.Fatal("fabricated evidence accepted")
	}
	for _, fn := range []func(context.Context, gitArgs) (toolOutput, error){tools.diff, tools.compare, tools.history, tools.blame, tools.historySearch} {
		o, e := fn(ctx, gitArgs{Path: "guard.any", Query: "authorize", Start: 1, End: 1})
		if e != nil || o.Error != "" || o.Text == "" {
			t.Fatalf("Git query: %+v %v", o, e)
		}
	}
	outside, _ := tools.file(ctx, readArgs{Path: "../config.yaml"})
	if outside.Error == "" {
		t.Fatal("path traversal accepted")
	}
}
func TestSearchCursorAndInvestigationLifecycle(t *testing.T) {
	g, s := localGitFixture(t)
	os.WriteFile(filepath.Join(g.Directory, "large.any"), []byte(strings.Repeat("needle "+strings.Repeat("x", 100)+"\n", 400)), 0600)
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "large"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = g.Directory
		if e := cmd.Run(); e != nil {
			t.Fatal(e)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.Directory
	raw, _ := cmd.Output()
	s.HeadSHA = strings.TrimSpace(string(raw))
	tools := toolsFor(g, s)
	a := searchArgs{Query: "needle", Path: "large.any"}
	var text strings.Builder
	for i := 0; i < 10; i++ {
		o, _ := tools.search(context.Background(), a)
		if o.Error != "" {
			t.Fatal(o)
		}
		text.WriteString(o.Text)
		if !o.More {
			break
		}
		a.Cursor = o.NextCursor
	}
	if strings.Count(text.String(), "needle") != 400 {
		t.Fatal("pagination lost results")
	}
	if len(tools.unresolved()) != 0 {
		t.Fatal("completed pages still marked incomplete")
	}
	o, _ := tools.record(context.Background(), Investigation{Claim: "guard may be removed"})
	var item Investigation
	json.Unmarshal([]byte(o.Text), &item)
	if item.ID == "" {
		t.Fatal(o)
	}
	item.Status = "rejected"
	o, _ = tools.update(context.Background(), item)
	if o.Error == "" {
		t.Fatal("rejection without counterevidence accepted")
	}
	source, _ := tools.file(context.Background(), readArgs{Path: "guard.any", Base: true})
	item.CounterObservationIDs = []string{source.ObservationID}
	item.Counterevidence = []string{"caller applies guard before invoking"}
	o, _ = tools.update(context.Background(), item)
	if o.Error != "" || tools.investigations()[0].Status != "rejected" {
		t.Fatal(o)
	}
	tools.maxCalls = tools.calls
	o, _ = tools.directory(context.Background(), directoryArgs{})
	if o.Error == "" {
		t.Fatal("call budget not enforced")
	}
}
func TestPrepareGitLabFetchesPinnedObjectsAndCleansWorkspace(t *testing.T) {
	g, s := localGitFixture(t)
	ctx := context.Background()
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":1,"http_url_to_repo":%q}`, baseURL+"/repo.git")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/repo.git/") {
			http.NotFound(w, r)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "oauth2" || password != "fixture-secret" {
			t.Error("Git fetch did not receive scoped authentication")
		}
		cmd := exec.CommandContext(r.Context(), "git", "http-backend")
		cmd.Env = append(os.Environ(), "GIT_PROJECT_ROOT="+g.Directory, "GIT_HTTP_EXPORT_ALL=1", "PATH_INFO=/"+strings.TrimPrefix(r.URL.Path, "/repo.git/"), "REQUEST_METHOD="+r.Method, "QUERY_STRING="+r.URL.RawQuery, "CONTENT_TYPE="+r.Header.Get("Content-Type"), fmt.Sprintf("CONTENT_LENGTH=%d", r.ContentLength))
		cmd.Stdin = r.Body
		// Export fixture .git through the smart HTTP CGI backend.
		cmd.Env = append(cmd.Env, "GIT_PROJECT_ROOT="+filepath.Dir(g.Directory), "PATH_INFO=/"+filepath.Base(g.Directory)+"/.git/"+strings.TrimPrefix(r.URL.Path, "/repo.git/"))
		raw, e := cmd.Output()
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		headers, body, ok := strings.Cut(string(raw), "\r\n\r\n")
		if !ok {
			headers, body, ok = strings.Cut(string(raw), "\n\n")
		}
		if !ok {
			t.Error("bad CGI response")
			return
		}
		for _, line := range strings.Split(headers, "\n") {
			k, v, ok := strings.Cut(line, ":")
			if ok {
				w.Header().Set(k, strings.TrimSpace(v))
			}
		}
		w.Write([]byte(body))
	}))
	defer server.Close()
	baseURL = server.URL
	remote, e := NewGitLabRepository("fixture-secret", baseURL)
	if e != nil {
		t.Fatal(e)
	}
	cfg := GitAuditSettings{HistoryDepth: 200, MaxPackMiB: 16, MaxToolCalls: 80}
	local, cleanup, e := PrepareGitLab(ctx, remote, s, cfg)
	if e != nil {
		t.Fatal(e)
	}
	dir := local.Directory
	defer cleanup()
	text, e := local.ReadFile(ctx, s, "guard.any", true)
	if e != nil || !strings.Contains(text, "authorize") {
		t.Fatal(text, e)
	}
	config, _ := os.ReadFile(filepath.Join(dir, "config"))
	if strings.Contains(string(config), "fixture-secret") || strings.Contains(string(config), "Authorization") {
		t.Fatal("credential written to config")
	}
	cleanup()
	if _, e = os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("workspace leaked")
	}
}

func TestStandaloneGitUsesEinoWithoutPlatformAPI(t *testing.T) {
	g, s := localGitFixture(t)
	var call int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Error("unexpected platform API request")
			http.NotFound(w, r)
			return
		}
		var req struct {
			Tools []any `json:"tools"`
		}
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Error(e)
		}
		if len(req.Tools) != 13 {
			t.Error("missing tools")
		}
		w.Header().Set("Content-Type", "application/json")
		call++
		names := []string{"list_directory", "search_code", "get_diff", "record_hypothesis", "update_investigation", "submit_finding"}
		finding := Finding{Side: "base", File: "guard.any", Line: 1, Severity: "high", Type: "authorization", Title: "guard removed", Description: "potential unauthorized access", Evidence: "authorize(user)", Trigger: "untrusted caller reaches operation", Suggestion: "restore guard", Confidence: "candidate"}
		args := []any{directoryArgs{Depth: 1}, searchArgs{Query: "authorize", Base: true}, gitArgs{Path: "guard.any"}, Investigation{ID: "guard", Claim: "guard removed", Evidence: []string{"base guard.any:1 authorize(user)"}, ObservationIDs: []string{"observation-2"}}, Investigation{ID: "guard", Claim: "guard removed", Status: "supported", Evidence: []string{"base guard.any:1 authorize(user)"}, ObservationIDs: []string{"observation-2"}, Counterevidence: []string{"No runtime verification performed"}}, finding}
		var msg map[string]any
		finish := "stop"
		if call <= len(names) {
			raw, _ := json.Marshal(args[call-1])
			finish = "tool_calls"
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": fmt.Sprintf("c%d", call), "type": "function", "function": map[string]any{"name": names[call-1], "arguments": string(raw)}}}}
		} else {
			raw, _ := json.Marshal(AuditResult{Findings: []Finding{finding}, Summary: "Guard removal requires review; not runtime verified", CoverageNotes: []string{}})
			msg = map[string]any{"role": "assistant", "content": string(raw)}
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}})
	}))
	defer server.Close()
	cfg := Settings{}
	cfg.OpenAI.URL = server.URL
	cfg.OpenAI.APIKey = "fixture-key"
	cfg.OpenAI.Model = "fixture-model"
	cfg.ReAct.MaxSteps = 20
	cfg.AuditTimeoutSeconds = 30
	defaultGitAudit(&cfg.GitAudit)
	run, e := AuditGit(context.Background(), g.Directory, s.BaseSHA, s.HeadSHA, "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	if len(run.Result.Findings) != 1 || len(run.Result.Investigations) != 1 || run.Result.Findings[0].ID == "" {
		t.Fatalf("missing validated evidence: %+v", run.Result)
	}
	if proof := os.Getenv("AIM_AUDIT_FIXTURE_PROOF"); proof != "" {
		raw, _ := json.Marshal(run)
		if e := os.WriteFile(proof, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if call != 7 {
		t.Fatalf("expected full investigation, got %d calls", call)
	}
	for _, tr := range run.Trace {
		if tr.Name != "model" && (tr.Output == "" || tr.ObservationID == "" || tr.Error != "") {
			t.Fatalf("invalid observation: %+v", tr)
		}
	}
}
func TestGitCommandCancellationBudgetsAndConfigValidation(t *testing.T) {
	g, s := localGitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := g.ReadFile(ctx, s, "guard.any", false); e == nil {
		t.Fatal("canceled read succeeded")
	}
	bad := s
	bad.HeadSHA = "--help"
	if _, e := g.ReadFile(context.Background(), bad, "guard.any", false); e == nil {
		t.Fatal("option injection accepted")
	}
	b := limitedBuffer{limit: 2}
	if _, e := b.Write([]byte("long")); e == nil {
		t.Fatal("output budget ignored")
	}
	cfg, e := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if e != nil {
		t.Fatal(e)
	}
	next := cfg.Snapshot()
	next.GitAudit.MaxToolCalls = 201
	if cfg.Save(next) == nil {
		t.Fatal("unsafe tool limit accepted")
	}
	next = cfg.Snapshot()
	next.GitAudit.HistoryDepth = 333
	if e = cfg.Save(next); e != nil {
		t.Fatal(e)
	}
	again, e := OpenSettings(cfg.path, "../../config.example.yaml")
	if e != nil || again.Snapshot().GitAudit.HistoryDepth != 333 {
		t.Fatal("Git settings not persisted")
	}
}

func TestSearchRejectsOversizedBlobBeforeGrep(t *testing.T) {
	raw := "100644 blob abc 20000000\tassets/large.any\x00100644 blob abc 20\tsrc/small.any\x00"
	if validateSearchSizes(raw, searchArgs{}) == nil {
		t.Fatal("oversized search input accepted")
	}
	if e := validateSearchSizes(raw, searchArgs{Path: "src"}); e != nil {
		t.Fatal(e)
	}
}
