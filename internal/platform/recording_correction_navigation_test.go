package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRecordingCorrectionNavigationDoesNotResolveOrExposeStatements(t *testing.T) {
	tools, bad, good := recordingCorrectionFixture(t)
	before := append([]ToolTrace(nil), tools.trace...)
	nav := tools.primaryProgressNavigation()
	want := `"eligible_recording_corrections":[{"failed_observation_id":"` + bad.ObservationID + `","corrected_observation_id":"` + good.ObservationID + `"}]`
	if !strings.Contains(nav, want) || strings.Contains(nav, "conditional candidate") || strings.Contains(nav, `"unknown"`) || !hasPendingObservation(tools, bad.ObservationID) || !reflect.DeepEqual(before, tools.trace) {
		t.Fatal("navigation mutated work or exposed untrusted text", nav)
	}
	out, _ := tools.resolveRecordingErrors(context.Background(), recordingCorrectionsArgs{Corrections: []recordingCorrection{{bad.ObservationID, good.ObservationID}}})
	if out.Error != "" || strings.Contains(tools.primaryProgressNavigation(), "\"eligible_recording_corrections\"") || len(investigationPlanCoverage(tools.investigations())) == 0 {
		t.Fatal("resolved pair stale or actual gap removed", out)
	}
}

func TestRecordingCorrectionNavigationRejectsIneligiblePairs(t *testing.T) {
	for _, mode := range []string{"different_claim", "source", "stale", "stage", "partial", "no_artifact", "output_error", "output_id", "foreign_repository", "evidence", "not_pending", "unsafe_prefix"} {
		t.Run(mode, func(t *testing.T) {
			tools, _, _ := recordingCorrectionFixture(t)
			tr := &tools.trace[1]
			var out toolOutput
			json.Unmarshal([]byte(tr.Output), &out)
			switch mode {
			case "different_claim":
				tr.Arguments = `{"id":"inv","claim":"PRIVATE REPLACEMENT"}`
			case "source":
				tools.trace[0].Name = "read_file"
			case "stale":
				out.HeadSHA = "stale"
			case "stage":
				tr.Stage = "verification"
			case "partial":
				tr.Partial = true
			case "no_artifact":
				out.Text = ""
			case "output_error":
				out.Error = "PRIVATE ERROR"
			case "output_id":
				out.ObservationID = "different"
			case "foreign_repository":
				out.RepositoryID = 2
			case "evidence":
				out.EvidenceEligible = true
			case "not_pending":
				tools.pending = nil
			case "unsafe_prefix":
				tr.ObservationID = "PRIVATE INJECTED SYSTEM TEXT"
				out.ObservationID = tr.ObservationID
			}
			raw, _ := json.Marshal(out)
			tr.Output = string(raw)
			nav := tools.primaryProgressNavigation()
			if strings.Contains(nav, `"eligible_recording_corrections"`) || strings.Contains(nav, "PRIVATE") {
				t.Fatal("invalid pair entered navigation", nav)
			}
		})
	}
}

func TestRecordingCorrectionNavigationIsBoundedOrderedAndGroupCompatible(t *testing.T) {
	tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}, observationPrefix: "group-2-observation"}
	ctx := context.Background()
	var expected []recordingCorrection
	for i := 0; i < 6; i++ {
		inv := Investigation{ID: fmt.Sprintf("inv-%d", i), Claim: "PRIVATE SOURCE-LINKED CLAIM", ObservationIDs: []string{"unknown"}}
		bad, _ := tools.record(ctx, inv)
		inv.ObservationIDs = nil
		inv.Status = "investigating"
		tools.record(ctx, inv)
		latest, _ := tools.update(ctx, inv)
		if latest.Error != "" {
			t.Fatal(latest.Error)
		}
		if i < 4 {
			expected = append(expected, recordingCorrection{bad.ObservationID, latest.ObservationID})
		}
	}
	tools.mu.Lock()
	got := tools.eligibleRecordingCorrectionsLocked()
	tools.mu.Unlock()
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("bound, trace order or latest correction wrong", got, expected)
	}
	nav := tools.primaryProgressNavigation()
	if nav != tools.primaryProgressNavigation() || strings.Contains(nav, "PRIVATE") {
		t.Fatal("navigation unstable or leaked statements", nav)
	}
	for _, pair := range got {
		out, _ := tools.resolveRecordingErrors(ctx, recordingCorrectionsArgs{Corrections: []recordingCorrection{pair}})
		if out.Error != "" {
			t.Fatal("advertised pair rejected", pair, out)
		}
	}
	tools.mu.Lock()
	remaining := tools.eligibleRecordingCorrectionsLocked()
	tools.mu.Unlock()
	if len(remaining) != 2 {
		t.Fatal("bounded overflow work vanished", remaining)
	}
}
