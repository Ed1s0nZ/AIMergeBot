package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func countTreeCommands(t *testing.T) string {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	count := filepath.Join(dir, "counts")
	script := "#!/bin/sh\ncase \" $* \" in *\" ls-tree \"*) printf 'tree\\n' >> " + strconv.Quote(count) + ";; esac\nexec " + strconv.Quote(realGit) + " \"$@\"\n"
	if err = os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return count
}
func treeCommandCount(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "tree\n")
}
func TestGitPathCacheCoalescesPinnedReadsAndCopiesResults(t *testing.T) {
	g, snap := localGitFixture(t)
	count := countTreeCommands(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.paths(ctx, snap, false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if treeCommandCount(t, count) != 1 {
		t.Fatal("same commit repeatedly queried Git")
	}
	paths, err := g.paths(ctx, snap, false)
	if err != nil {
		t.Fatal(err)
	}
	paths[0] = "mutated"
	again, err := g.paths(ctx, snap, false)
	if err != nil || again[0] == "mutated" {
		t.Fatal("caller modified cache")
	}
	if _, err = g.paths(ctx, snap, true); err != nil {
		t.Fatal(err)
	}
	if treeCommandCount(t, count) != 2 {
		t.Fatal("BASE and HEAD were mixed")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = g.paths(ctx, snap, false); err == nil {
		t.Fatal("cancelled cache read succeeded")
	}
}
func TestGitPathCacheKeysDirectoryAndSHAAndBoundsEntries(t *testing.T) {
	g, snap := localGitFixture(t)
	other, otherSnap := localGitFixture(t)
	count := countTreeCommands(t)
	ctx := context.Background()
	if _, err := g.paths(ctx, snap, false); err != nil {
		t.Fatal(err)
	}
	if _, err := g.paths(ctx, snap, true); err != nil {
		t.Fatal(err)
	}
	g.Directory = other.Directory
	if _, err := g.paths(ctx, otherSnap, false); err != nil {
		t.Fatal(err)
	}
	if treeCommandCount(t, count) != 3 || len(g.pathCache) != 2 || g.pathCacheBytes > gitPathCacheLimit {
		t.Fatal("directory isolation or bounds failed")
	}
	// Invalid objects never become cache entries.
	bad := otherSnap
	bad.HeadSHA = strings.Repeat("0", 40)
	if _, err := g.paths(ctx, bad, false); err == nil {
		t.Fatal("invalid object succeeded")
	}
	if len(g.pathCache) != 2 {
		t.Fatal("failure cached")
	}
}

func TestGitPathCacheRejectsOversizedListingWithoutPartialResults(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "listing")
	// A small raw output with many paths also consumes string-header memory.
	if err := os.WriteFile(raw, []byte(strings.Repeat("a\x00", 600000)), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nexec /bin/cat " + strconv.Quote(raw) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := &GitRepository{Directory: dir}
	snap := Snapshot{HeadSHA: strings.Repeat("a", 40)}
	paths, err := g.paths(context.Background(), snap, false)
	if err == nil || paths != nil || len(g.pathCache) != 0 || g.pathCacheBytes != 0 {
		t.Fatalf("oversize returned/cached partial paths: %d %v", len(paths), err)
	}
}
