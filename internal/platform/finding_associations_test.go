package platform

import (
	"fmt"
	"strings"
	"testing"
)

func associationFixture() (Run, Run) {
	old := Run{ID: 1, Status: "succeeded", Snapshot: Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}}
	f := Finding{ID: "old", File: "old.any", Line: 2, Side: "head", Type: "authorization", Evidence: "danger(input)", Trigger: "untrusted input"}
	f.Fingerprint = findingFingerprint(old.Snapshot, f)
	old.Result.Findings = []Finding{f}
	current := Run{ID: 2, Status: "succeeded", Snapshot: old.Snapshot}
	current.HeadSHA = strings.Repeat("c", 40)
	f.ID, f.File, f.Line = "current", "new.any", 20
	f.Fingerprint = findingFingerprint(current.Snapshot, f)
	current.Result.Findings = []Finding{f}
	current.Result.MetadataChanges = []GitChangeMetadata{{Kind: "rename", OldPath: "old.any", NewPath: "new.any", Base: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("d", 40)}, Head: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("e", 40)}}}
	return current, old
}

func TestAssociationSuggestionCapsCandidatesAndMarksReverseAmbiguity(t *testing.T) {
	current, old := associationFixture()
	f, prior := current.Result.Findings[0], old.Result.Findings[0]
	current.Result.Findings = nil
	old.Result.Findings = nil
	for i := 0; i < 55; i++ {
		f.ID = fmt.Sprintf("current-%02d", i)
		prior.ID = fmt.Sprintf("prior-%02d", i)
		f.Evidence = fmt.Sprintf("danger(%d)", i)
		prior.Evidence = f.Evidence
		f.Fingerprint = findingFingerprint(current.Snapshot, f)
		prior.Fingerprint = findingFingerprint(old.Snapshot, prior)
		current.Result.Findings = append(current.Result.Findings, f)
		old.Result.Findings = append(old.Result.Findings, prior)
	}
	out := suggestFindingAssociations(current, []Run{old})
	if !out.Truncated || len(out.Items) != 50 || len(out.Limitations) < 2 {
		t.Fatal("candidate cap hidden", len(out.Items), out.Truncated)
	}
	current, old = associationFixture()
	f = current.Result.Findings[0]
	f.ID = "another"
	current.Result.Findings = append(current.Result.Findings, f)
	out = suggestFindingAssociations(current, []Run{old})
	if len(out.Items) != 2 || !out.Items[0].Ambiguous || !out.Items[1].Ambiguous {
		t.Fatal("one historical issue claimed by two current anchors without warning")
	}
	clearAmbiguousFingerprints(current.Result.Findings)
	if out = suggestFindingAssociations(current, []Run{old}); len(out.Items) != 0 {
		t.Fatal("cleared ambiguous source identity reused")
	}
	if !strings.Contains(strings.Join(out.Limitations, " "), "fingerprint") {
		t.Fatal("ambiguous source omission not disclosed")
	}
}
func TestAssociationSuggestionIsAnExplicitHintWithoutChangingIdentity(t *testing.T) {
	current, old := associationFixture()
	before := current.Result.Findings[0].Fingerprint
	out := suggestFindingAssociations(current, []Run{old})
	if len(out.Items) != 1 || out.Items[0].State.Decision != "pending" || out.Items[0].State.Revision != 0 || out.Items[0].OldPath != "old.any" || out.Items[0].NewPath != "new.any" || len(out.Limitations) == 0 {
		t.Fatal("missing explicit hint", out)
	}
	if current.Result.Findings[0].Fingerprint != before || before == old.Result.Findings[0].Fingerprint {
		t.Fatal("identity merged")
	}
	other := old
	other.ID = 2
	current.ID = 3
	out = suggestFindingAssociations(current, []Run{old, other})
	if len(out.Items) != 2 || !out.Items[0].Ambiguous || !out.Items[1].Ambiguous {
		t.Fatal("multiple versions silently chosen")
	}
	stable := suggestFindingAssociations(current, []Run{other, old})
	if stable.Items[0].ID != out.Items[0].ID || stable.Items[1].ID != out.Items[1].ID {
		t.Fatal("order changed candidate identity")
	}
}
func TestAssociationSuggestionRejectsUnrelatedFactsOrScope(t *testing.T) {
	for _, mode := range []string{"type", "evidence", "trigger", "base", "metadata_anchor", "fingerprint", "project", "source", "mr", "context", "running", "copy", "symlink", "gitlink", "no_metadata"} {
		t.Run(mode, func(t *testing.T) {
			current, old := associationFixture()
			f := &old.Result.Findings[0]
			switch mode {
			case "type":
				f.Type = "other"
			case "evidence":
				f.Evidence = "similar but different"
			case "trigger":
				f.Trigger = "trusted input"
			case "base":
				f.Side = "base"
			case "metadata_anchor":
				f.AnchorType = "git_metadata"
			case "fingerprint":
				f.Fingerprint = ""
			case "project":
				old.ProjectID = 2
			case "source":
				old.SourceProjectID = 2
			case "mr":
				old.MRIID = 2
			case "context":
				old.AuditPolicy = &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("a", 40)}}}
			case "running":
				current.Status = "running"
			case "copy":
				current.Result.MetadataChanges[0].Kind = "copy"
			case "symlink":
				current.Result.MetadataChanges[0].Head.Mode = "120000"
			case "gitlink":
				current.Result.MetadataChanges[0].Head.Mode = "160000"
				current.Result.MetadataChanges[0].Head.Type = "commit"
			case "no_metadata":
				current.Result.MetadataChanges = nil
			}
			if mode != "fingerprint" {
				f.Fingerprint = findingFingerprint(old.Snapshot, *f)
			}
			if got := suggestFindingAssociations(current, []Run{old}); len(got.Items) != 0 {
				t.Fatal("unrelated suggestion accepted", mode)
			}
		})
	}
}
