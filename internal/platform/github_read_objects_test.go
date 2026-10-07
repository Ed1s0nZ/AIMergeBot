package platform

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var githubFixtureCommit = strings.Repeat("1", 40)
var githubFixtureRoot = strings.Repeat("2", 40)
var githubFixtureSubtree = strings.Repeat("3", 40)

func githubBlobID(raw []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(raw))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func githubObjectJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func githubTreeNode(name, mode, sha string, size int) map[string]any {
	kind := entryType(mode)
	if mode == "040000" {
		kind = "tree"
	}
	node := map[string]any{"path": name, "mode": mode, "type": kind, "sha": sha}
	if kind == "blob" {
		node["size"] = size
	}
	return node
}
func githubTreeJSON(w http.ResponseWriter, sha string, nodes []map[string]any) {
	githubObjectJSON(w, map[string]any{"sha": sha, "truncated": false, "tree": nodes})
}
func githubBlobJSON(w http.ResponseWriter, raw []byte) {
	githubObjectJSON(w, map[string]any{"sha": githubBlobID(raw), "encoding": "base64", "size": len(raw), "content": base64.StdEncoding.EncodeToString(raw), "url": "https://ignored.example/fixture"})
}
func githubObjectsFixture(t *testing.T, handler http.HandlerFunc) (*githubObjectReader, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/Org/Repo" {
			githubObjectJSON(w, map[string]any{"id": 7, "full_name": "Org/Repo"})
			return
		}
		calls.Add(1)
		if r.URL.RawQuery != "" {
			t.Error("object reader unexpectedly enabled recursive/pagination query")
		}
		handler(w, r)
	})
	reader, err := newGitHubObjectReader(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	return reader, &calls
}
func githubCommitJSON(w http.ResponseWriter, commit, root string) {
	githubObjectJSON(w, map[string]any{"sha": commit, "tree": map[string]any{"sha": root}})
}
func githubObjectErrorKind(t *testing.T, err error, kind string) {
	t.Helper()
	var failure *githubReadError
	if !errors.As(err, &failure) || failure.kind != kind {
		t.Fatalf("wanted %s, got %v", kind, err)
	}
}

func TestGitHubObjectsFixedSHAUnicodeCacheAndBlobIdentity(t *testing.T) {
	raw := []byte("package 固定\n")
	blob := githubBlobID(raw)
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Org/Repo/git/commits/" + githubFixtureCommit:
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		case "/repos/Org/Repo/git/trees/" + githubFixtureRoot:
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("目录 空格", "040000", githubFixtureSubtree, 0)})
		case "/repos/Org/Repo/git/trees/" + githubFixtureSubtree:
			githubTreeJSON(w, githubFixtureSubtree, []map[string]any{githubTreeNode("代码.go", "100755", blob, len(raw))})
		case "/repos/Org/Repo/git/blobs/" + blob:
			githubBlobJSON(w, raw)
		default:
			t.Error("reader accessed an unpinned or arbitrary URL")
			w.WriteHeader(404)
		}
	})
	for i := 0; i < 2; i++ {
		got, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "目录 空格/代码.go")
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("fixed file read failed: %v", err)
		}
	}
	if calls.Load() != 5 {
		t.Fatal("commit/tree cache failed or unexpected route accessed")
	}
	entries, err := reader.directory(context.Background(), githubFixtureCommit, "目录 空格")
	if err != nil {
		t.Fatal(err)
	}
	entries[0].Path = "tampered"
	entries[0].Entry.ObjectID = githubFixtureRoot
	again, err := reader.directory(context.Background(), githubFixtureCommit, "目录 空格")
	if err != nil || again[0].Path != "代码.go" || again[0].Entry.ObjectID != blob {
		t.Fatal("caller mutated cached directory")
	}
	if calls.Load() != 5 {
		t.Fatal("directory re-read immutable objects")
	}
}
func TestGitHubObjectsInvalidInputHasNoObjectRequests(t *testing.T) {
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input made object request") })
	for _, sha := range []string{"main", "1234", strings.Repeat("a", 64), strings.Repeat("0", 40), strings.Repeat("A", 40)} {
		if _, err := reader.readFileBytes(context.Background(), sha, "file"); err == nil {
			t.Error("invalid SHA accepted")
		}
	}
	for _, path := range []string{"", "/file", "../file", "a/../file", "a//file", "a/", "a\x00b", "a\nb", "\xff", strings.Repeat("a", 256), strings.Repeat("a/", 64) + "file"} {
		if _, err := reader.readFileBytes(context.Background(), githubFixtureCommit, path); err == nil {
			t.Error("invalid path accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input issued HTTP")
	}
}
func TestGitHubObjectsCommitIdentityAndTreeShapeFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		commit any
		tree   any
		kind   string
	}{
		{"commit_drift", map[string]any{"sha": githubFixtureSubtree, "tree": map[string]any{"sha": githubFixtureRoot}}, nil, "object_identity_changed"},
		{"root_missing", map[string]any{"sha": githubFixtureCommit}, nil, "object_identity_changed"},
		{"truncated", nil, map[string]any{"sha": githubFixtureRoot, "truncated": true, "tree": []any{}}, "tree_incomplete"},
		{"unknown_complete", nil, map[string]any{"sha": githubFixtureRoot, "tree": []any{}}, "invalid_tree_response"},
		{"null_tree", nil, map[string]any{"sha": githubFixtureRoot, "truncated": false, "tree": nil}, "invalid_tree_response"},
		{"tree_drift", nil, map[string]any{"sha": githubFixtureSubtree, "truncated": false, "tree": []any{}}, "invalid_tree_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					if tc.commit != nil {
						githubObjectJSON(w, tc.commit)
					} else {
						githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
					}
				} else {
					githubObjectJSON(w, tc.tree)
				}
			})
			_, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "missing")
			githubObjectErrorKind(t, err, tc.kind)
			if len(reader.trees) != 0 {
				t.Fatal("incomplete tree cached")
			}
		})
	}
}
func TestGitHubObjectsMalformedDirectoryAndBudgets(t *testing.T) {
	base := githubTreeNode("file", "100644", githubFixtureSubtree, 1)
	for _, tc := range []struct {
		name        string
		nodes       []map[string]any
		seedEntries int
		kind        string
	}{
		{"duplicate", []map[string]any{base, base}, 0, "invalid_tree_response"},
		{"nested_name", []map[string]any{githubTreeNode("a/b", "100644", githubFixtureSubtree, 1)}, 0, "invalid_tree_response"},
		{"unknown_mode", []map[string]any{githubTreeNode("file", "100600", githubFixtureSubtree, 1)}, 0, "invalid_tree_response"},
		{"missing_size", []map[string]any{{"path": "file", "mode": "100644", "type": "blob", "sha": githubFixtureSubtree}}, 0, "invalid_tree_response"},
		{"negative_size", []map[string]any{githubTreeNode("file", "100644", githubFixtureSubtree, -1)}, 0, "invalid_tree_response"},
		{"entry_cache", []map[string]any{base}, githubObjectEntryLimit, "tree_budget_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
				} else {
					githubTreeJSON(w, githubFixtureRoot, tc.nodes)
				}
			})
			reader.entries = tc.seedEntries
			_, err := reader.directory(context.Background(), githubFixtureCommit, "")
			githubObjectErrorKind(t, err, tc.kind)
			if len(reader.trees) != 0 {
				t.Fatal("invalid entries cached")
			}
		})
	}
	nodes := make([]map[string]any, 2001)
	for i := range nodes {
		nodes[i] = githubTreeNode(fmt.Sprintf("f%d", i), "100644", githubFixtureSubtree, 0)
	}
	reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		} else {
			githubTreeJSON(w, githubFixtureRoot, nodes)
		}
	})
	_, err := reader.directory(context.Background(), githubFixtureCommit, "")
	githubObjectErrorKind(t, err, "tree_budget_exceeded")
}
func TestGitHubObjectsRejectNonRegularOrLargeFilesBeforeBlob(t *testing.T) {
	for _, mode := range []string{"120000", "160000", "040000", "100644"} {
		t.Run(mode, func(t *testing.T) {
			var blobs atomic.Int32
			reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
				} else if strings.Contains(r.URL.Path, "/trees/") {
					githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("file", mode, githubFixtureSubtree, githubFileReadLimit+1)})
				} else {
					blobs.Add(1)
				}
			})
			_, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "file")
			if mode == "100644" {
				githubObjectErrorKind(t, err, "file_read_budget_exceeded")
			} else {
				githubObjectErrorKind(t, err, "not_a_regular_file")
			}
			if blobs.Load() != 0 {
				t.Fatal("unsafe file issued blob request")
			}
		})
	}
}
func TestGitHubObjectsBlobValidationEmptyBinaryAndBoundary(t *testing.T) {
	// Independent Git empty-blob object ID keeps the fixture hash helper honest.
	if githubBlobID(nil) != "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391" {
		t.Fatal("fixture does not match the Git empty-blob identity")
	}
	for _, raw := range [][]byte{{}, {0, 255, 1}, bytes.Repeat([]byte("x"), githubFileReadLimit)} {
		raw := raw
		t.Run(fmt.Sprint(len(raw)), func(t *testing.T) {
			reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/commits/") {
					githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
				} else if strings.Contains(r.URL.Path, "/trees/") {
					githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("file", "100644", githubBlobID(raw), len(raw))})
				} else {
					githubBlobJSON(w, raw)
				}
			})
			got, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "file")
			if err != nil || !bytes.Equal(got, raw) {
				t.Fatalf("blob boundary/bytes not preserved: %v", err)
			}
		})
	}
	for _, mutate := range []func(map[string]any){func(v map[string]any) { v["content"] = "%%%" }, func(v map[string]any) { v["encoding"] = "utf-8" }, func(v map[string]any) { delete(v, "content") }, func(v map[string]any) { v["size"] = 2 }, func(v map[string]any) { v["sha"] = githubFixtureRoot }, func(v map[string]any) { v["content"] = base64.StdEncoding.EncodeToString([]byte("bad")) }} {
		reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
			raw := []byte("yes")
			if strings.Contains(r.URL.Path, "/commits/") {
				githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
			} else if strings.Contains(r.URL.Path, "/trees/") {
				githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("file", "100644", githubBlobID(raw), len(raw))})
			} else {
				v := map[string]any{"sha": githubBlobID(raw), "encoding": "base64", "size": len(raw), "content": base64.StdEncoding.EncodeToString(raw)}
				mutate(v)
				githubObjectJSON(w, v)
			}
		})
		if _, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "file"); err == nil {
			t.Fatal("invalid blob accepted")
		}
	}
}
func TestGitHubObjectsAbsenceOnlyFromCompleteDirectory(t *testing.T) {
	reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		} else {
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{})
		}
	})
	_, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "missing")
	githubObjectErrorKind(t, err, "path_absent")
	for _, status := range []int{404, 429} {
		reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		_, err := reader.readFileBytes(context.Background(), githubFixtureCommit, "missing")
		if err == nil {
			t.Fatal("upstream failure treated as success")
		}
		var failure *githubReadError
		if errors.As(err, &failure) && failure.kind == "path_absent" {
			t.Fatal("upstream failure established absence")
		}
		if status == 429 && UpstreamFailureInfo(err) == nil {
			t.Fatal("rate metadata lost")
		}
	}
}
func TestGitHubObjectsWaitingGateCanBeCanceled(t *testing.T) {
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("canceled gate made request") })
	if err := reader.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer reader.release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := reader.directory(ctx, githubFixtureCommit, "")
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 0 {
		t.Fatal("waiting gate ignored cancellation")
	}
}

func TestGitHubObjectsReadDistinctHistoricalCommits(t *testing.T) {
	head := strings.Repeat("4", 40)
	headRoot := strings.Repeat("5", 40)
	before, after := []byte("old code\n"), []byte("new code\n")
	reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Org/Repo/git/commits/" + githubFixtureCommit:
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		case "/repos/Org/Repo/git/commits/" + head:
			githubCommitJSON(w, head, headRoot)
		case "/repos/Org/Repo/git/trees/" + githubFixtureRoot:
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("file", "100644", githubBlobID(before), len(before))})
		case "/repos/Org/Repo/git/trees/" + headRoot:
			githubTreeJSON(w, headRoot, []map[string]any{githubTreeNode("file", "100644", githubBlobID(after), len(after))})
		case "/repos/Org/Repo/git/blobs/" + githubBlobID(before):
			githubBlobJSON(w, before)
		case "/repos/Org/Repo/git/blobs/" + githubBlobID(after):
			githubBlobJSON(w, after)
		default:
			t.Error("historical read used a dynamic branch route")
			w.WriteHeader(404)
		}
	})
	for _, tc := range []struct {
		sha      string
		expected []byte
	}{{githubFixtureCommit, before}, {head, after}, {githubFixtureCommit, before}} {
		raw, err := reader.readFileBytes(context.Background(), tc.sha, "file")
		if err != nil || !bytes.Equal(raw, tc.expected) {
			t.Fatalf("historical commit was reinterpreted: %v", err)
		}
	}
}
