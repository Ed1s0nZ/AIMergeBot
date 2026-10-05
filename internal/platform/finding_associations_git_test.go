package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssociationSuggestionUsesActualGitMoveAndFixedSourceFacts(t *testing.T) {
	g, snap := localGitFixture(t)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = g.Directory
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("fixture git", err)
		}
		return strings.TrimSpace(string(raw))
	}
	base := snap.HeadSHA
	text := strings.Repeat("stable context\n", 30) + "danger(input)\n"
	if err := os.WriteFile(filepath.Join(g.Directory, "guard.any"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "guard.any")
	git("commit", "-m", "historical risk fixture")
	oldHead := git("rev-parse", "HEAD")
	if err := os.Mkdir(filepath.Join(g.Directory, "moved"), 0700); err != nil {
		t.Fatal(err)
	}
	git("mv", "guard.any", "moved/guard.any")
	if err := os.WriteFile(filepath.Join(g.Directory, "moved/guard.any"), []byte(text+"danger(input)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "moved/guard.any")
	git("commit", "-m", "move with repeated changed anchor")
	head := git("rev-parse", "HEAD")
	current, old := associationFixture()
	old.BaseSHA, old.HeadSHA = base, oldHead
	old.Result.Findings[0].File = "guard.any"
	old.Result.Findings[0].Line = 31
	old.Result.Findings[0].Fingerprint = findingFingerprint(old.Snapshot, old.Result.Findings[0])
	current.BaseSHA, current.HeadSHA = oldHead, head
	current.Result.Findings[0].File = "moved/guard.any"
	current.Result.Findings[0].Line = 32
	current.Result.Findings[0].Fingerprint = findingFingerprint(current.Snapshot, current.Result.Findings[0])
	changes, notes, err := g.Changes(context.Background(), current.Snapshot)
	if err != nil || len(notes) != 0 {
		t.Fatal("fixed native changes", err, notes)
	}
	current.Result.MetadataChanges = nil
	for _, change := range changes {
		if change.Metadata != nil {
			current.Result.MetadataChanges = append(current.Result.MetadataChanges, *change.Metadata)
		}
	}
	out := suggestFindingAssociations(current, []Run{old})
	if len(out.Items) != 1 || out.Items[0].Metadata.Kind != "rename" || out.Items[0].OldPath != "guard.any" || out.Items[0].NewPath != "moved/guard.any" || out.Items[0].PriorHeadSHA != oldHead || out.Items[0].HeadSHA != head {
		t.Fatal("actual git move not represented", out)
	}
	scope := BuildDiff(changes, nil, 96000)
	if !scope.Added["moved/guard.any"][32] {
		t.Fatal("current fixture is not a changed HEAD anchor")
	}
	for _, run := range []Run{old, current} {
		f := run.Result.Findings[0]
		content, err := g.ReadFile(context.Background(), run.Snapshot, f.File, false)
		if err != nil || strings.Split(content, "\n")[f.Line-1] != f.Evidence {
			t.Fatal("fixed source fact mismatch", err)
		}
	}
}
