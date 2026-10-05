package platform

import (
	"context"
	"testing"
)

func TestToolEvidenceEligibilityAndDiffIrrelevantLimit(t *testing.T) {
	repo, snap := localGitFixture(t)
	tools := &auditTools{repo: repo, snap: snap, cache: map[string]string{}}
	diff, _ := tools.diff(context.Background(), gitArgs{Limit: 200})
	if diff.Error != "" || !diff.EvidenceEligible {
		t.Fatal("diff rejected unused history limit", diff.Error)
	}
	history, _ := tools.history(context.Background(), gitArgs{Limit: 201})
	if history.Error == "" || history.EvidenceEligible || len(history.EligibleObservationIDs) != 1 || history.EligibleObservationIDs[0] != diff.ObservationID {
		t.Fatal("invalid history limit or source hint")
	}
	listing, _ := tools.list(context.Background(), listArgs{Page: 1})
	if listing.EvidenceEligible {
		t.Fatal("enumeration became source proof")
	}
	tools.mu.Lock()
	err := tools.validateObservationIDs([]string{listing.ObservationID})
	tools.mu.Unlock()
	if err == nil {
		t.Fatal("directory proof accepted")
	}
}
