package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusPinnedRepositoriesAreDeterministicAndBlind(t *testing.T) {
	corpus, digest, err := LoadCorpus("../../evaluation/corpus-v1.json")
	if err != nil || len(digest) != 64 || len(corpus.Cases) != 11 {
		t.Fatal(corpus.Version, digest, err)
	}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			first := filepath.Join(t.TempDir(), "repo")
			second := filepath.Join(t.TempDir(), "repo")
			base, head, err := BuildRepository(context.Background(), first, c)
			if err != nil {
				t.Fatal(err)
			}
			otherBase, otherHead, err := BuildRepository(context.Background(), second, c)
			if err != nil || base != otherBase || head != otherHead || base == head {
				t.Fatal("non-reproducible pinned fixture", err)
			}
			for name, expected := range c.HeadFiles {
				raw, err := os.ReadFile(filepath.Join(first, name))
				if err != nil || string(raw) != expected || strings.Contains(string(raw), c.Rationale) {
					t.Fatal("ground truth contamination", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(first, "corpus-v1.json")); !os.IsNotExist(err) {
				t.Fatal("ground truth entered repository")
			}
		})
	}
}
func TestFixtureBuilderRejectsTraversalAndGitConfiguration(t *testing.T) {
	for _, p := range []string{"../outside", "/tmp/outside", ".git/config", ".GIT/config", "sub/.git/hooks/pre-commit", "a/../../outside"} {
		root := filepath.Join(t.TempDir(), "repo")
		_, _, err := BuildRepository(context.Background(), root, Case{BaseFiles: map[string]string{p: "untrusted"}})
		if err == nil {
			t.Fatal("unsafe fixture path accepted", p)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("builder mutated before validation")
		}
	}
}
