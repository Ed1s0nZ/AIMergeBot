package evaluation

import (
	"context"
	"os"
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
