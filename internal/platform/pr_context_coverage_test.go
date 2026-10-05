package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPRCoveragePreservesFactsAndExposesMissingChain(t *testing.T) {
	f := Finding{ID: "finding-1", PRContext: &PRInvestigationContext{Before: "old", After: "new"}, Verification: &FindingVerification{Status: "supported", ClaimCoverage: "full"}}
	before, _ := json.Marshal(f)
	notes := findingPRCoverage([]Finding{f})
	if len(notes) != 3 || !strings.Contains(strings.Join(notes, " "), "finding-1") {
		t.Fatal("missing chain certified", notes)
	}
	after, _ := json.Marshal(f)
	if string(before) != string(after) {
		t.Fatal("projection mutated model facts")
	}
	f.PRContext.BeforeObservationIDs = []string{"base"}
	f.PRContext.AfterObservationIDs = []string{"head"}
	f.PRContext.Relationships = []InvestigationRelationship{{From: "entry", To: "sink", Relation: "call", Certainty: "cited", ObservationIDs: []string{"head"}}}
	if len(findingPRCoverage([]Finding{f})) != 0 {
		t.Fatal("required source/relationship fields marked missing")
	}
	f.PRContext.Relationships[0].Certainty = "inferred"
	notes = findingPRCoverage([]Finding{f})
	if len(notes) != 1 || !strings.Contains(notes[0], "unverified") {
		t.Fatal("inferred relationship silently certified", notes)
	}
	f.PRContext = nil
	if len(findingPRCoverage([]Finding{f})) != 1 {
		t.Fatal("absent structure hidden")
	}
	if len(findingPRCoverage(nil)) != 0 {
		t.Fatal("no findings created gaps")
	}
}

func TestMetadataPRCoverageDoesNotInventExecutionPath(t *testing.T) {
	f := Finding{ID: "metadata", AnchorType: "git_metadata", PRContext: &PRInvestigationContext{BeforeObservationIDs: []string{"diff"}, AfterObservationIDs: []string{"diff"}}}
	if len(findingPRCoverage([]Finding{f})) != 0 {
		t.Fatal("metadata-only claim required a runtime path")
	}
	f.PRContext.AfterObservationIDs = nil
	if notes := findingPRCoverage([]Finding{f}); len(notes) != 1 || !strings.Contains(notes[0], "HEAD") {
		t.Fatal("metadata comparison gap hidden", notes)
	}
}

func TestCanonicalMetadataAlreadyContainsSnapshotComparison(t *testing.T) {
	f := Finding{AnchorType: "git_metadata", Metadata: &GitChangeMetadata{}}
	if len(findingPRCoverage([]Finding{f})) != 0 {
		t.Fatal("canonical metadata comparison ignored")
	}
}
