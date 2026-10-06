package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func claimSourceFixture() (*auditTools, Investigation, claimVerificationInput) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	fresh := &auditTools{snap: snap, stage: claimVerificationStage, observationPrefix: "claim-digest-observation", scope: DiffScope{Included: []string{"entry.any"}}}
	for n, base := range []bool{true, false} {
		id := []string{"claim-digest-observation-1", "claim-digest-observation-2"}[n]
		args, _ := json.Marshal(readArgs{Path: "entry.any", Base: base, Start: 1, End: 2})
		out, _ := json.Marshal(toolOutput{ObservationID: id, EvidenceEligible: true, BaseSHA: "base", HeadSHA: "head", Text: "1: fixed conditional guard"})
		fresh.trace = append(fresh.trace, ToolTrace{Name: "read_file", Stage: claimVerificationStage, ObservationID: id, Arguments: string(args), Output: string(out)})
	}
	return fresh, Investigation{Claim: "HEAD retains compatibility", Status: "rejected"}, claimVerificationInput{Verdict: "true", Reason: "Source comparison supports compatibility", Limitations: []string{}, ObservationIDs: []string{fresh.trace[0].ObservationID, fresh.trace[1].ObservationID}}
}

func TestClaimVerificationFixedContextAndCanonicalMetadata(t *testing.T) {
	fresh, item, input := claimSourceFixture()
	fresh.snap.AuditPolicy = &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "ctx"}}}
	id := fresh.observationPrefix + "-3"
	out, _ := json.Marshal(toolOutput{ObservationID: id, EvidenceEligible: true, RepositoryID: 2, BaseSHA: "ctx", HeadSHA: "ctx", Text: "1: downstream default is bounded"})
	fresh.trace = append(fresh.trace, ToolTrace{Name: "read_repository_file", Stage: claimVerificationStage, ObservationID: id, Output: string(out)})
	input.ObservationIDs = append(input.ObservationIDs, id)
	v, err := validateClaimVerification(input, item, fresh)
	if err != nil || v.Status != "disagreed" {
		t.Fatal("complete fixed context rejected", v, err)
	}
	// Actual source tool registration produces eligible canonical Git facts.
	m := GitChangeMetadata{Kind: "modify", OldPath: "task.any", NewPath: "task.any", Base: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("a", 40)}, Head: &GitEntry{Mode: "100755", Type: "blob", ObjectID: strings.Repeat("a", 40)}}
	meta := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}, stage: claimVerificationStage, observationPrefix: "claim-meta-observation", scope: DiffScope{Metadata: map[string]GitChangeMetadata{"task.any": m}}}
	read, err := meta.changeMetadata(context.Background(), metadataArgs{Path: "task.any"})
	if err != nil || read.Error != "" {
		t.Fatal(read, err)
	}
	input.ObservationIDs = []string{read.ObservationID}
	v, err = validateClaimVerification(input, item, meta)
	if err != nil || v.Status != "disagreed" {
		t.Fatal(v, err)
	}
	// Metadata accompanying a body change cannot replace source comparison.
	meta.scope.Metadata["task.any"] = GitChangeMetadata{Kind: "modify", OldPath: "task.any", NewPath: "task.any", Base: m.Base, Head: &GitEntry{Mode: "100755", Type: "blob", ObjectID: strings.Repeat("d", 40)}}
	v, err = validateClaimVerification(input, item, meta)
	if err != nil || v.Status != "inconclusive" {
		t.Fatal("noncanonical/body-change metadata accepted", v, err)
	}
}

func TestClaimVerificationFreshSourceBoundary(t *testing.T) {
	fresh, item, input := claimSourceFixture()
	v, err := validateClaimVerification(input, item, fresh)
	if err != nil || v.Status != "disagreed" || v.AssessedClaim != item.Claim || v.BaseSHA != "base" || item.Status != "rejected" {
		t.Fatal(v, err)
	}
	for _, mode := range []string{"primary", "other_review", "duplicate", "read_failed", "directory", "snapshot", "ineligible", "id_mismatch", "partial", "empty", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			f, i, in := claimSourceFixture()
			tr := &f.trace[0]
			var out toolOutput
			json.Unmarshal([]byte(tr.Output), &out)
			switch mode {
			case "primary":
				tr.Stage = "primary"
			case "other_review":
				in.ObservationIDs[0] = "claim-other-observation-1"
			case "duplicate":
				f.trace = append(f.trace, *tr)
			case "read_failed":
				tr.Error = "failed"
			case "directory":
				tr.Name = "list_files"
			case "snapshot":
				out.HeadSHA = "latest"
			case "ineligible":
				out.EvidenceEligible = false
			case "id_mismatch":
				out.ObservationID = "primary-1"
			case "partial":
				out.More = true
			case "empty":
				out.Text = " "
			}
			raw, _ := json.Marshal(out)
			tr.Output = string(raw)
			if mode == "malformed" {
				tr.Output = "{"
			}
			if _, err := validateClaimVerification(in, i, f); err == nil {
				t.Fatal("invalid evidence accepted", mode)
			}
		})
	}
}

func TestClaimVerificationMissingSideOrContextRemainsUnknown(t *testing.T) {
	for _, mode := range []string{"no_base", "unchanged_base", "context_only", "missing_context", "no_sources"} {
		fresh, item, input := claimSourceFixture()
		switch mode {
		case "no_base":
			input.ObservationIDs = input.ObservationIDs[1:]
		case "unchanged_base":
			fresh.trace[0].Arguments = `{"path":"unchanged.any","base":true}`
		case "context_only":
			fresh.snap.AuditPolicy = &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "ctx"}}}
			for n := range fresh.trace {
				var out toolOutput
				json.Unmarshal([]byte(fresh.trace[n].Output), &out)
				out.RepositoryID, out.BaseSHA, out.HeadSHA = 2, "ctx", "ctx"
				raw, _ := json.Marshal(out)
				fresh.trace[n].Output = string(raw)
				fresh.trace[n].Name = "read_repository_file"
			}
		case "missing_context":
			fresh.snap.AuditPolicy = &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "ctx"}}}
		case "no_sources":
			input.ObservationIDs = []string{}
		}
		v, err := validateClaimVerification(input, item, fresh)
		if err != nil || v.Status != "inconclusive" || v.Verdict != "unknown" || len(v.Limitations) == 0 || item.Status != "rejected" {
			t.Fatal(mode, v, err)
		}
	}
	// Unknown is an honest independent outcome even without decisive source.
	fresh, item, input := claimSourceFixture()
	input.Verdict, input.ObservationIDs = "unknown", []string{}
	v, err := validateClaimVerification(input, item, fresh)
	if err != nil || v.Status != "inconclusive" {
		t.Fatal(v, err)
	}
}

func TestClaimVerificationCompareUsesActualCanonicalBasePath(t *testing.T) {
	for _, tc := range []struct {
		old            string
		rename, accept bool
	}{
		{"", false, true}, {"entry.any", false, true}, {"unrelated.any", false, false},
		{"old.any", true, true}, {"unrelated.any", true, false}, {"", true, false},
	} {
		fresh, item, input := claimSourceFixture()
		fresh.trace = fresh.trace[:1]
		fresh.trace[0].Name = "compare_files"
		args, _ := json.Marshal(gitArgs{Path: "entry.any", OldPath: tc.old})
		fresh.trace[0].Arguments = string(args)
		input.ObservationIDs = input.ObservationIDs[:1]
		if tc.rename {
			fresh.scope.Metadata = map[string]GitChangeMetadata{"entry.any": {Kind: "rename", OldPath: "old.any", NewPath: "entry.any", Base: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("a", 40)}, Head: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("b", 40)}}}
		}
		v, err := validateClaimVerification(input, item, fresh)
		if err != nil || (v.Status == "disagreed") != tc.accept {
			t.Fatal(tc, v, err)
		}
	}
	// Exercise the real fixed Git producer; both blobs exist but are unrelated.
	repo, snap := localGitFixture(t)
	fresh := &auditTools{repo: repo, snap: snap, stage: claimVerificationStage, observationPrefix: "claim-git-observation", scope: DiffScope{Included: []string{"file-000.unknown", "guard.any"}}}
	for _, tc := range []struct {
		path, old string
		accept    bool
	}{
		{"file-000.unknown", "guard.any", false}, {"guard.any", "", true},
	} {
		out, err := fresh.compare(context.Background(), gitArgs{Path: tc.path, OldPath: tc.old})
		if err != nil || out.Error != "" || !out.EvidenceEligible {
			t.Fatal("controlled compare did not produce actual source", out, err)
		}
		input := claimVerificationInput{Verdict: "true", Reason: "bounded fixture", Limitations: []string{}, ObservationIDs: []string{out.ObservationID}}
		v, err := validateClaimVerification(input, Investigation{Claim: "same claim", Status: "rejected"}, fresh)
		if err != nil || (v.Status == "disagreed") != tc.accept {
			t.Fatal(tc, v, err)
		}
	}
}

func TestClaimVerificationGroupedAnchorsSupplySides(t *testing.T) {
	fresh, item, input := claimSourceFixture()
	fresh.scope = DiffScope{Added: map[string]map[int]bool{"entry.any": {1: true}}}
	v, err := validateClaimVerification(input, item, fresh)
	if err != nil || v.Status != "disagreed" {
		t.Fatal(v, err)
	}
}
