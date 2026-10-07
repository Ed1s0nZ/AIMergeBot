package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGitHubListingStablePagesSharedTreesAndCopies(t *testing.T) {
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Org/Repo/git/commits/" + githubFixtureCommit:
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		case "/repos/Org/Repo/git/trees/" + githubFixtureRoot:
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("b", "040000", githubFixtureSubtree, 0), githubTreeNode("a", "040000", githubFixtureSubtree, 0), githubTreeNode("link", "120000", strings.Repeat("4", 40), 1), githubTreeNode("module", "160000", strings.Repeat("5", 40), 0)})
		case "/repos/Org/Repo/git/trees/" + githubFixtureSubtree:
			nodes := []map[string]any{}
			for i := 102; i >= 0; i-- {
				nodes = append(nodes, githubTreeNode(fmt.Sprintf("f%03d", i), "100644", strings.Repeat("4", 40), 0))
			}
			githubTreeJSON(w, githubFixtureSubtree, nodes)
		default:
			t.Error("listing accessed an unexpected object")
			w.WriteHeader(404)
		}
	})
	first, more, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	if err != nil || !more || len(first) != 100 || first[0] != "a/f000" || first[99] != "a/f099" {
		t.Fatalf("first page invalid: %v", err)
	}
	first[0] = "tampered"
	second, more, err := reader.listFiles(context.Background(), githubFixtureCommit, 2)
	if err != nil || !more || len(second) != 100 || second[0] != "a/f100" {
		t.Fatal("second page unstable")
	}
	third, more, err := reader.listFiles(context.Background(), githubFixtureCommit, 3)
	if err != nil || more || len(third) != 6 || third[5] != "b/f102" {
		t.Fatal("last page invalid")
	}
	first, _, err = reader.listFiles(context.Background(), githubFixtureCommit, 1)
	if err != nil || first[0] != "a/f000" || calls.Load() != 3 {
		t.Fatal("immutable listing cache mutated or shared tree fetched repeatedly")
	}
	var wg sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
			if err != nil || len(p) != 100 || p[0] != "a/f000" {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 || calls.Load() != 3 {
		t.Fatal("concurrent cached listing changed")
	}
	empty, more, err := reader.listFiles(context.Background(), githubFixtureCommit, 20)
	if err != nil || more || len(empty) != 0 {
		t.Fatal("valid page past end not empty")
	}
	for _, page := range []int{0, 21} {
		if _, _, err := reader.listFiles(context.Background(), githubFixtureCommit, page); err == nil {
			t.Fatal("invalid page accepted")
		}
	}
}
func TestGitHubListingCycleCannotEstablishEmptyRepository(t *testing.T) {
	reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		} else {
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("loop", "040000", githubFixtureRoot, 0)})
		}
	})
	paths, _, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	githubObjectErrorKind(t, err, "invalid_tree_cycle")
	if paths != nil || len(reader.listings) != 0 {
		t.Fatal("cyclic directory produced/cached successful partial listing")
	}
	_, err = reader.readFileBytes(context.Background(), githubFixtureCommit, "loop/file")
	githubObjectErrorKind(t, err, "invalid_tree_cycle")
}
func TestGitHubListingTruncationRetryDoesNotCachePartial(t *testing.T) {
	var complete atomic.Bool
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Org/Repo/git/commits/" + githubFixtureCommit:
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		case "/repos/Org/Repo/git/trees/" + githubFixtureRoot:
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("dir", "040000", githubFixtureSubtree, 0)})
		case "/repos/Org/Repo/git/trees/" + githubFixtureSubtree:
			githubObjectJSON(w, map[string]any{"sha": githubFixtureSubtree, "truncated": !complete.Load(), "tree": []map[string]any{githubTreeNode("file", "100644", strings.Repeat("4", 40), 0)}})
		default:
			w.WriteHeader(404)
		}
	})
	paths, _, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	githubObjectErrorKind(t, err, "tree_incomplete")
	if paths != nil || len(reader.listings) != 0 {
		t.Fatal("truncated list produced partial success")
	}
	complete.Store(true)
	paths, more, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	if err != nil || more || len(paths) != 1 || paths[0] != "dir/file" || calls.Load() != 4 {
		t.Fatal("explicit retry used a cached partial tree")
	}
}
func TestGitHubListingFileAndCacheBudgetsAreExplicit(t *testing.T) {
	reader, _ := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
			return
		}
		if strings.HasSuffix(r.URL.Path, githubFixtureRoot) {
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("a", "040000", githubFixtureSubtree, 0), githubTreeNode("b", "040000", strings.Repeat("4", 40), 0)})
			return
		}
		sha := githubFixtureSubtree
		count := 1000
		if strings.HasSuffix(r.URL.Path, strings.Repeat("4", 40)) {
			sha = strings.Repeat("4", 40)
			count = 1001
		}
		nodes := []map[string]any{}
		for i := 0; i < count; i++ {
			nodes = append(nodes, githubTreeNode(fmt.Sprintf("f%d", i), "100644", strings.Repeat("5", 40), 0))
		}
		githubTreeJSON(w, sha, nodes)
	})
	paths, _, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	githubObjectErrorKind(t, err, "file_listing_budget_exceeded")
	if paths != nil || len(reader.listings) != 0 {
		t.Fatal("over-budget list succeeded partially")
	}
	reader, _ = githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
		} else {
			githubTreeJSON(w, githubFixtureRoot, []map[string]any{githubTreeNode("file", "100644", githubFixtureSubtree, 0)})
		}
	})
	reader.listingBytes = githubListingPathByteLimit
	_, _, err = reader.listFiles(context.Background(), githubFixtureCommit, 1)
	githubObjectErrorKind(t, err, "file_listing_budget_exceeded")
	if len(reader.listings) != 0 {
		t.Fatal("oversized path cache saved")
	}
}
func TestGitHubListingSharedDAGHasExpansionBudget(t *testing.T) {
	reader, calls := githubObjectsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/commits/") {
			githubCommitJSON(w, githubFixtureCommit, githubFixtureRoot)
			return
		}
		sha := strings.TrimPrefix(r.URL.Path, "/repos/Org/Repo/git/trees/")
		level := 0
		if sha != githubFixtureRoot {
			if _, err := fmt.Sscanf(sha, "%x", new(uint64)); err != nil {
				t.Error("invalid test object route")
			}
			for i := 1; i <= 15; i++ {
				if sha == fmt.Sprintf("%040x", 100+i) {
					level = i
					break
				}
			}
		}
		if level == 15 {
			githubTreeJSON(w, sha, []map[string]any{})
			return
		}
		next := fmt.Sprintf("%040x", 101+level)
		githubTreeJSON(w, sha, []map[string]any{githubTreeNode("a", "040000", next, 0), githubTreeNode("b", "040000", next, 0)})
	})
	paths, _, err := reader.listFiles(context.Background(), githubFixtureCommit, 1)
	githubObjectErrorKind(t, err, "file_listing_budget_exceeded")
	if paths != nil || len(reader.listings) != 0 || calls.Load() > 17 {
		t.Fatal("DAG expansion escaped budget or refetched cached objects")
	}
}
