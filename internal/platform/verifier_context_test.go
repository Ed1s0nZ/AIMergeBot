package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestClaimReviewNavigationIsBoundedAndNotPrimaryJudgment(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	parent := &auditTools{repo: repo, snap: snap, cache: map[string]string{}, scope: DiffScope{Included: []string{f.File}}}
	out, _ := parent.file(context.Background(), readArgs{Path: f.File, Start: 1, End: 2})
	item := Investigation{Claim: "actual claim", Status: "rejected", Evidence: []string{"FIRST_REVIEW_SECRET"}, ObservationIDs: []string{out.ObservationID}}
	n := buildClaimReviewNavigation(parent, item)
	raw, _ := json.Marshal(n)
	if len(n.Sources) != 1 || n.Omitted || strings.Contains(string(raw), "FIRST_REVIEW_SECRET") || strings.Contains(string(raw), "danger(input)") || strings.Contains(string(raw), out.ObservationID) {
		t.Fatal("primary judgment/source/IDs leaked or valid locator lost", string(raw))
	}
	item.CounterObservationIDs = []string{"unlocated"}
	if !buildClaimReviewNavigation(parent, item).Omitted {
		t.Fatal("missing locators concealed")
	}
	for i := 0; i < 30; i++ {
		parent.scope.Included = append(parent.scope.Included, fmt.Sprintf("path-%02d.any", i))
	}
	n = buildClaimReviewNavigation(parent, item)
	if len(n.ChangedPaths) != 20 || !n.Omitted {
		t.Fatal("path budget or omission disclosure", n)
	}
	parent.trace[0].Stage = claimVerificationStage
	if n := buildClaimReviewNavigation(parent, item); len(n.Sources) != 0 {
		t.Fatal("supplemental source reused as primary locator", n)
	}
}

func TestClaimReviewNamespacesKeepArtifactBoundaries(t *testing.T) {
	a := Investigation{ID: "a", Claim: "b\x00c"}
	b := Investigation{ID: "a\x00b", Claim: "c"}
	if claimObservationPrefix(a) == claimObservationPrefix(b) {
		t.Fatal("artifact/claim boundary collision")
	}
	if claimObservationPrefix(a) != claimObservationPrefix(a) || len(claimObservationPrefix(a)) > 150 {
		t.Fatal("unstable or oversized prefix")
	}
	if claimObservationPrefix(Investigation{ID: "group-a-1", Claim: "same"}) == claimObservationPrefix(Investigation{ID: "group-b-1", Claim: "same"}) {
		t.Fatal("group namespaces merged")
	}
}
