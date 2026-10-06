package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestUnknownInvestigationFeedbackScopedBoundedAndEncoded(t *testing.T) {
	ledger := map[string]Investigation{"z": {Claim: "private claim"}, `a"<script>`: {}}
	msg := unknownInvestigationError(ledger).Error()
	var ids []string
	if err := json.NewDecoder(strings.NewReader(strings.SplitN(msg, "known_investigation_ids=", 2)[1])).Decode(&ids); err != nil || len(ids) != 2 || ids[0] != `a"<script>` || ids[1] != "z" {
		t.Fatal(ids, err)
	}
	if strings.Contains(msg, "private claim") {
		t.Fatal("claim exposed")
	}
	for _, l := range []map[string]Investigation{nil, {"": {}}, {" ": {}}, {strings.Repeat("x", 81): {}}, {"nul\x00": {}}, {string([]byte{0xff}): {}}} {
		if strings.Contains(unknownInvestigationError(l).Error(), "known_investigation_ids=") {
			t.Fatal("invalid/empty history exposed")
		}
	}
	large := map[string]Investigation{}
	for n := 0; n < 31; n++ {
		large[fmt.Sprint(n)] = Investigation{}
	}
	if strings.Contains(unknownInvestigationError(large).Error(), "known_investigation_ids=") {
		t.Fatal("too many identities exposed")
	}
	large = map[string]Investigation{}
	for n := 0; n < 30; n++ {
		large[fmt.Sprint(n)+strings.Repeat("\x01", 70)] = Investigation{}
	}
	if strings.Contains(unknownInvestigationError(large).Error(), "known_investigation_ids=") {
		t.Fatal("oversized encoded data exposed")
	}
}

func TestUnknownInvestigationNeverSelectsOrRetiresAmbiguousArtifact(t *testing.T) {
	tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}}
	for _, id := range []string{"one", "two"} {
		out, _ := tools.record(context.Background(), Investigation{ID: id, Claim: "same statement"})
		if out.Error != "" {
			t.Fatal(out)
		}
	}
	before, _ := json.Marshal(tools.investigations())
	for _, id := range []string{"", "foreign"} {
		bad, _ := tools.update(context.Background(), Investigation{ID: id, Claim: "same statement", Status: "investigating"})
		if bad.Error == "" || !strings.Contains(bad.Error, `known_investigation_ids=["one","two"]`) || bad.EvidenceEligible {
			t.Fatal(bad)
		}
		after, _ := json.Marshal(tools.investigations())
		if string(before) != string(after) {
			t.Fatal("implicitly selected record")
		}
		good, _ := tools.update(context.Background(), Investigation{ID: "one", Claim: "same statement", Status: "investigating"})
		if good.Error != "" || !hasPendingObservation(tools, bad.ObservationID) {
			t.Fatal("valid explicit update failed or bad unknown identity silently retired", good)
		}
		retired, _ := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: []recordingCorrection{{bad.ObservationID, good.ObservationID}}})
		if retired.Error == "" || !hasPendingObservation(tools, bad.ObservationID) {
			t.Fatal("cross-identity correction accepted")
		}
		if strings.Contains(tools.primaryProgressNavigation(), "known_investigation_ids") {
			t.Fatal("ids in trusted guidance")
		}
		out, _ := tools.update(context.Background(), Investigation{ID: "one", Claim: "same statement", Status: "supported", Evidence: []string{"feedback"}, ObservationIDs: []string{bad.ObservationID}})
		if out.Error == "" {
			t.Fatal("feedback promoted to source")
		}
	}
}
