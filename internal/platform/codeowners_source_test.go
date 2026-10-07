package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type codeOwnerSourceFixture struct {
	fixtureRepo
	provider       string
	dirs           map[string][]CodeOwnerEntry
	content        map[string]string
	directoryError error
	readError      error
	reads          []string
	snap           Snapshot
}

func (f *codeOwnerSourceFixture) CodeOwnersProvider() string { return f.provider }
func (f *codeOwnerSourceFixture) CodeOwnerDirectory(_ context.Context, s Snapshot, dir string) ([]CodeOwnerEntry, error) {
	if s.HeadSHA != f.snap.HeadSHA || s.SourceProjectID != f.snap.SourceProjectID {
		return nil, errors.New("wrong snapshot")
	}
	if f.directoryError != nil {
		return nil, f.directoryError
	}
	return f.dirs[dir], nil
}
func (f *codeOwnerSourceFixture) ReadFile(_ context.Context, s Snapshot, path string, base bool) (string, error) {
	f.reads = append(f.reads, path)
	if base || s.HeadSHA != f.snap.HeadSHA || s.SourceProjectID != f.snap.SourceProjectID {
		return "", errors.New("wrong snapshot")
	}
	return f.content[path], f.readError
}
func TestCodeOwnerSourceProviderPriorityAndFixedForkHead(t *testing.T) {
	snap := Snapshot{ProjectID: 1, SourceProjectID: 7, HeadSHA: strings.Repeat("a", 40)}
	for _, provider := range []string{"github", "gitlab"} {
		f := &codeOwnerSourceFixture{provider: provider, snap: snap, dirs: map[string][]CodeOwnerEntry{
			"":     {{"CODEOWNERS", "100644", "blob"}, {"docs", "040000", "tree"}, {".github", "040000", "tree"}, {".gitlab", "040000", "tree"}},
			"docs": {{"docs/CODEOWNERS", "100644", "blob"}}, ".github": {{".github/CODEOWNERS", "100644", "blob"}}, ".gitlab": {{".gitlab/CODEOWNERS", "100644", "blob"}},
		}, content: map[string]string{"CODEOWNERS": "* @root", ".github/CODEOWNERS": "* @github", "docs/CODEOWNERS": "* @docs"}}
		doc, err := LoadCodeOwnerDocument(context.Background(), f, snap)
		want := "CODEOWNERS"
		if provider == "github" {
			want = ".github/CODEOWNERS"
		}
		if err != nil || doc.Path != want || doc.RepositoryID != 7 || doc.SHA != snap.HeadSHA || !doc.Present || len(f.reads) != 1 {
			t.Fatal(provider, doc, f.reads, err)
		}
		match, err := doc.Rules.Match("source.any")
		if err != nil || len(match.Rules) != 1 {
			t.Fatal(match, err)
		}
	}
}
func TestCodeOwnerSourceAbsenceDiffersFromUnavailableAndInvalid(t *testing.T) {
	snap := Snapshot{SourceProjectID: 7, HeadSHA: strings.Repeat("a", 40)}
	for _, tc := range []struct {
		name                      string
		root                      []CodeOwnerEntry
		raw                       string
		directoryError, readError error
		wantErr, wantPresent      bool
	}{
		{name: "absent"},
		{name: "listing incomplete", directoryError: ErrCodeOwnersSource, wantErr: true},
		{name: "listing denied", directoryError: errors.New("denied"), wantErr: true},
		{name: "read missing after known-present", root: []CodeOwnerEntry{{"CODEOWNERS", "100644", "blob"}}, readError: errors.New("404"), wantErr: true, wantPresent: true},
		{name: "symlink", root: []CodeOwnerEntry{{"CODEOWNERS", "120000", "blob"}}, wantErr: true, wantPresent: true},
		{name: "bad rules", root: []CodeOwnerEntry{{"CODEOWNERS", "100644", "blob"}}, raw: "[broken", wantErr: true, wantPresent: true},
		{name: "oversize", root: []CodeOwnerEntry{{"CODEOWNERS", "100644", "blob"}}, raw: strings.Repeat("x", 256*1024+1), wantErr: true, wantPresent: true},
		{name: "empty file", root: []CodeOwnerEntry{{"CODEOWNERS", "100644", "blob"}}, wantPresent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &codeOwnerSourceFixture{provider: "gitlab", snap: snap, dirs: map[string][]CodeOwnerEntry{"": tc.root}, content: map[string]string{"CODEOWNERS": tc.raw}, directoryError: tc.directoryError, readError: tc.readError}
			doc, err := LoadCodeOwnerDocument(context.Background(), f, snap)
			if (err != nil) != tc.wantErr || doc.Present != tc.wantPresent {
				t.Fatal(doc, err)
			}
			if tc.wantErr && doc.Rules != nil {
				t.Fatal("partial rules returned")
			}
		})
	}
}
func TestGitLabCodeOwnerDirectoryUsesFixedSourceAndRejectsIncompletePages(t *testing.T) {
	snap := Snapshot{ProjectID: 1, SourceProjectID: 7, HeadSHA: strings.Repeat("a", 40)}
	mode := "success"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.URL.Path, "/api/v4/projects/7/repository/") || r.URL.Query().Get("ref") != snap.HeadSHA {
			t.Error("not fixed fork HEAD", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/tree") {
			if mode == "denied" {
				http.Error(w, "denied", 403)
				return
			}
			if mode == "pages" {
				nodes := []map[string]string{}
				for i := 0; i < 100; i++ {
					nodes = append(nodes, map[string]string{"path": fmt.Sprintf("file-%s-%d", r.URL.Query().Get("page"), i), "mode": "100644", "type": "blob"})
				}
				json.NewEncoder(w).Encode(nodes)
				return
			}
			fmt.Fprint(w, `[{"path":"CODEOWNERS","mode":"100644","type":"blob"}]`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/files/CODEOWNERS") {
			fmt.Fprintf(w, `{"size":8,"content":%q}`, base64.StdEncoding.EncodeToString([]byte("* @owner")))
			return
		}
		t.Error("unexpected request", r.URL)
		http.NotFound(w, r)
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("fixture", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := LoadCodeOwnerDocument(context.Background(), repo, snap)
	if err != nil || !doc.Present || doc.Path != "CODEOWNERS" || calls != 2 {
		t.Fatal(doc, calls, err)
	}
	mode = "denied"
	calls = 0
	if _, err := LoadCodeOwnerDocument(context.Background(), repo, snap); err == nil || calls != 1 {
		t.Fatal("denied treated absent", calls, err)
	}
	mode = "pages"
	calls = 0
	if _, err := LoadCodeOwnerDocument(context.Background(), repo, snap); !errors.Is(err, ErrCodeOwnersSource) || calls != 20 {
		t.Fatal("pagination treated complete", calls, err)
	}
}

func TestCodeOwnerLocalGitReadsPinnedObjectsAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		raw, err := command.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(raw))
		}
		return strings.TrimSpace(string(raw))
	}
	git("init", "--initial-branch=main")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.test")
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "CODEOWNERS"), []byte("* @pinned"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "pinned rules")
	head := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "docs", "CODEOWNERS"), []byte("* @newer"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "newer rules")
	repo := &GitRepository{Directory: dir, Metadata: &codeOwnerSourceFixture{provider: "gitlab"}}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 7, HeadSHA: head}
	doc, err := LoadCodeOwnerDocument(context.Background(), repo, snap)
	if err != nil || doc.Path != "docs/CODEOWNERS" || doc.SHA != head {
		t.Fatal(doc, err)
	}
	match, err := doc.Rules.Match("source.any")
	if err != nil || len(match.Rules) != 1 || match.Rules[0].Owners[0] != "@pinned" {
		t.Fatal("used newer working tree", match, err)
	}
	if err := os.Symlink("docs/CODEOWNERS", filepath.Join(dir, "CODEOWNERS")); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "symlink rules")
	snap.HeadSHA = git("rev-parse", "HEAD")
	doc, err = LoadCodeOwnerDocument(context.Background(), repo, snap)
	if !errors.Is(err, ErrCodeOwnersSource) || doc.Path != "CODEOWNERS" || doc.Rules != nil {
		t.Fatal("symlink followed or lower priority fallback", doc, err)
	}
	snap.HeadSHA = strings.Repeat("0", 40)
	if _, err := LoadCodeOwnerDocument(context.Background(), repo, snap); !errors.Is(err, ErrCodeOwnersSource) {
		t.Fatal("invalid commit accepted", err)
	}
	repo.Metadata = nil
	snap.HeadSHA = head
	if _, err := LoadCodeOwnerDocument(context.Background(), repo, snap); !errors.Is(err, ErrCodeOwnersSource) {
		t.Fatal("unknown provider guessed", err)
	}
}
