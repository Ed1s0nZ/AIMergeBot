package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"pr_agent/internal/platform"
	"strings"
	"testing"
)

func TestHistoricalPRReadsPinnedObjectsWithoutMutatingRepository(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "source")
	base, head, err := BuildRepository(ctx, dir, Case{BaseFiles: map[string]string{"file.any": "old\n"}, HeadFiles: map[string]string{"file.any": "new\n"}})
	if err != nil {
		t.Fatal(err)
	}
	// Uncommitted working tree is not the audited snapshot.
	if err = os.WriteFile(filepath.Join(dir, "file.any"), []byte("dirty uncommitted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	item := Case{ID: "case-001", Expectation: "negative", Rationale: "private-ground-truth-marker", ExpectedAnchor: Anchor{File: "file.any", Side: "head", Line: 1}, Git: &PinnedGitCase{Directory: dir, BaseSHA: base, HeadSHA: head}}
	corpus := Corpus{Version: 1, Kind: "real-git-history-not-representative-benchmark", Cases: []Case{item}}
	path := filepath.Join(t.TempDir(), "ground-truth.json")
	raw, _ := json.Marshal(corpus)
	os.WriteFile(path, raw, 0600)
	loaded, _, err := LoadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	repo, b, h, err := PrepareCase(ctx, filepath.Join(t.TempDir(), "unused"), loaded.Cases[0])
	if err != nil || b != base || h != head {
		t.Fatal(err)
	}
	content, err := repo.ReadFile(ctx, platform.Snapshot{BaseSHA: b, HeadSHA: h}, "file.any", false)
	if err != nil || !strings.Contains(content, "new") || strings.Contains(content, item.Rationale) || strings.Contains(content, "dirty") {
		t.Fatal("not blind pinned read", err)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "file.any"))
	if err != nil || string(raw) != "dirty uncommitted\n" {
		t.Fatal("working tree changed")
	}
	for _, sha := range []string{"HEAD", strings.Repeat("0", 40)} {
		bad := item
		pin := *item.Git
		pin.HeadSHA = sha
		bad.Git = &pin
		if _, _, _, err = PrepareCase(ctx, "unused", bad); err == nil {
			t.Fatal("bad commit accepted")
		}
	}
	sub := filepath.Join(dir, "nested")
	if err = os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	nested := item
	pin := *item.Git
	pin.Directory = sub
	nested.Git = &pin
	if _, _, _, err = PrepareCase(ctx, "", nested); err == nil {
		t.Fatal("nested directory bypassed repository-root isolation")
	}
	if err = ValidateEvaluationLocation(corpus, filepath.Join(dir, "results")); err == nil {
		t.Fatal("labels enter audited root")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err = os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if err = ValidateEvaluationLocation(corpus, filepath.Join(alias, "new", "results")); err == nil {
		t.Fatal("symlink bypass")
	}
	if err = ValidateEvaluationLocation(corpus, filepath.Join(t.TempDir(), "results")); err != nil {
		t.Fatal(err)
	}
}
func TestHistoricalCorpusRejectsMixedFixture(t *testing.T) {
	dir := t.TempDir()
	corpus := Corpus{Version: 1, Kind: "real-git-history-not-representative-benchmark", Cases: []Case{{ID: "case-001", Expectation: "positive", Git: &PinnedGitCase{Directory: dir, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, BaseFiles: map[string]string{"a": "x"}}}}
	path := filepath.Join(t.TempDir(), "corpus.json")
	raw, _ := json.Marshal(corpus)
	os.WriteFile(path, raw, 0600)
	if _, _, err := LoadCorpus(path); err == nil {
		t.Fatal("mixed input accepted")
	}
}
