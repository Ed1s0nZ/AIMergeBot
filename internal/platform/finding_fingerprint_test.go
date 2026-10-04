package platform

import (
	"strings"
	"testing"
)

func TestFindingFingerprintConservativeCrossAttemptIdentity(t *testing.T) {
	snap := Snapshot{ProjectID: 1, MRIID: 2, SourceProjectID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	finding := Finding{File: "service.any", Side: "head", Line: 2, Type: "authorization", Title: "Original title", Evidence: "delete(resource)", Trigger: "request controlled by another user"}
	original := findingFingerprint(snap, finding)
	if len(original) != 64 {
		t.Fatal("missing stable identity")
	}
	next := snap
	next.HeadSHA = strings.Repeat("c", 40)
	moved := finding
	moved.Line = 20
	moved.Title = "Rephrased title"
	if findingFingerprint(next, moved) != original {
		t.Fatal("line/title/head changes lost association")
	}
	for _, change := range []func(*Finding){func(f *Finding) { f.File = "other.any" }, func(f *Finding) { f.Side = "base" }, func(f *Finding) { f.Type = "other risk" }, func(f *Finding) { f.Trigger = "different trigger" }, func(f *Finding) { f.Evidence = "guarded_delete(resource)" }} {
		different := finding
		change(&different)
		if findingFingerprint(snap, different) == original {
			t.Fatal("unsafe semantic association", different)
		}
	}
	for _, change := range []func(*Snapshot){func(s *Snapshot) { s.ProjectID = 9 }, func(s *Snapshot) { s.MRIID = 9 }, func(s *Snapshot) { s.SourceProjectID = 9 }} {
		different := snap
		change(&different)
		if findingFingerprint(different, finding) == original {
			t.Fatal("cross-scope association")
		}
	}
	snap.HeadSHA = ""
	if findingFingerprint(snap, finding) != "" {
		t.Fatal("unpinned legacy finding associated")
	}
}

func TestFingerprintCollisionDoesNotMergeUnrelatedAnchors(t *testing.T) {
	findings := []Finding{{ID: "a", Fingerprint: "same"}, {ID: "b", Fingerprint: "same"}, {ID: "c", Fingerprint: "unique"}}
	clearAmbiguousFingerprints(findings)
	if findings[0].Fingerprint != "" || findings[1].Fingerprint != "" || findings[2].Fingerprint != "unique" {
		t.Fatal("ambiguous anchors associated", findings)
	}
}
