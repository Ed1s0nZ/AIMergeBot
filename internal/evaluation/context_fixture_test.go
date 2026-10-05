package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestContextFixturesPinnedAndIsolated(t *testing.T) {
	c := Case{ContextRepositories: []ContextFixture{{RepositoryID: 2, Files: map[string]string{"svc.go": "package svc\n"}}}, Rationale: "outside-only-ground-truth"}
	first, policy, err := PrepareContextFixtures(context.Background(), t.TempDir(), c)
	if err != nil || len(policy) != 1 {
		t.Fatal(err)
	}
	_, other, err := PrepareContextFixtures(context.Background(), t.TempDir(), c)
	if err != nil || policy[0].SHA != other[0].SHA {
		t.Fatal("context not reproducible", err)
	}
	src := first[2]
	text, err := src.Repository.ReadFile(context.Background(), src.Snapshot, "svc.go", false)
	if err != nil || text != "package svc\n" || src.Snapshot.HeadSHA != policy[0].SHA {
		t.Fatal("not pinned", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if src.Authorize(ctx) == nil {
		t.Fatal("cancel ignored")
	}
}
func TestContextFixturesRejectBeforeMutation(t *testing.T) {
	for _, c := range []Case{
		{ContextRepositories: []ContextFixture{{RepositoryID: 1, Files: map[string]string{"a": "x"}}}},
		{ContextRepositories: []ContextFixture{{RepositoryID: 2, Files: map[string]string{"../a": "x"}}}},
		{ContextRepositories: []ContextFixture{{RepositoryID: 2, Files: map[string]string{".git/config": "x"}}}},
		{ContextRepositories: []ContextFixture{{RepositoryID: 2, Files: map[string]string{"a": "x"}}, {RepositoryID: 2, Files: map[string]string{"b": "x"}}}},
	} {
		root := t.TempDir()
		if _, _, err := PrepareContextFixtures(context.Background(), root, c); err == nil {
			t.Fatal("unsafe context accepted")
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Fatal("mutation before validation")
		}
	}
}

func TestCrossRepositoryCorpusPairsNeedContextEvidence(t *testing.T) {
	corpus, _, err := LoadCorpus("../../evaluation/corpus-cross-repository-v1.json")
	if err != nil || len(corpus.Cases) != 4 {
		t.Fatal("cross corpus", err)
	}
	for i := 0; i < len(corpus.Cases); i += 2 {
		positive, negative := corpus.Cases[i], corpus.Cases[i+1]
		if positive.Expectation != "positive" || negative.Expectation != "negative" {
			t.Fatal("pair labels")
		}
		a, _ := json.Marshal(positive.BaseFiles)
		b, _ := json.Marshal(negative.BaseFiles)
		h, _ := json.Marshal(positive.HeadFiles)
		j, _ := json.Marshal(negative.HeadFiles)
		if string(a) != string(b) || string(h) != string(j) {
			t.Fatal("paired PR differs; context is no longer decisive")
		}
	}
	for _, c := range corpus.Cases {
		root := t.TempDir()
		_, _, err := PrepareContextFixtures(context.Background(), root, c)
		if err != nil {
			t.Fatal(c.ID, err)
		}
		_, _, err = BuildRepository(context.Background(), filepath.Join(root, "primary"), c)
		if err != nil {
			t.Fatal(c.ID, err)
		}
	}
}
