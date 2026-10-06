package platform

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestClaimReviewCheckpointPreservesCancellationAndRecovery(t *testing.T) {
	for _, terminal := range []string{"cancelled", "failed"} {
		t.Run(terminal, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
			id, _, err := s.Enqueue(ctx, snap, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			result := AuditResult{Summary: "primary checkpoint", Investigations: []Investigation{{ID: "g1-inv1", Claim: "guard preserved", Status: "supported", Evidence: []string{"original evidence"}, ObservationIDs: []string{"primary-1"}}}}
			before, _ := json.Marshal(result)
			progress := claimReviewProgress(func(r AuditResult, tr []ToolTrace) error { return s.Checkpoint(ctx, id, r, tr) }, snap, true)
			trace := []ToolTrace{{ObservationID: "primary-1", Name: "read_file", Output: "original source"}}
			if err = progress(result, trace); err != nil {
				t.Fatal(err)
			}
			if terminal == "cancelled" {
				err = s.Cancel(ctx, id, 1)
			} else {
				err = s.Recover(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			completed := result
			completed.Investigations = append([]Investigation(nil), result.Investigations...)
			completed.Investigations[0].ClaimVerification = &ClaimVerification{Status: "consistent", Verdict: "true"}
			if err = progress(completed, trace); !errors.Is(err, ErrConflict) {
				t.Fatalf("late result escaped fence: %v", err)
			}
			run, err := s.Run(ctx, id)
			if err != nil || run.Status != terminal {
				t.Fatal(err, run)
			}
			item := run.Result.Investigations[0]
			v := item.ClaimVerification
			if v == nil || v.Status != "unavailable" || v.Verdict != "" || v.AssessedClaim != result.Investigations[0].Claim || v.BaseSHA != "base" || v.HeadSHA != "head" || len(v.ObservationIDs) != 0 {
				t.Fatal("interrupted review lost or fabricated", item)
			}
			if item.ID != "g1-inv1" || item.Status != "supported" || !reflect.DeepEqual(item.Evidence, []string{"original evidence"}) || !reflect.DeepEqual(run.Trace, trace) || run.Result.Summary != result.Summary {
				t.Fatal("primary checkpoint changed", run)
			}
			after, _ := json.Marshal(result)
			if string(before) != string(after) {
				t.Fatal("projection mutated live primary result")
			}
			export := BuildSARIF(run, nil)["runs"].([]sarifObject)[0]
			if export["invocations"].([]sarifObject)[0]["executionSuccessful"] != false {
				t.Fatal("interruption exported as success")
			}
			reviews := export["properties"].(sarifObject)["investigationClaimReviews"].([]sarifObject)
			if len(reviews) != 1 || reviews[0]["verification"].(*ClaimVerification).Status != "unavailable" {
				t.Fatal("SARIF review missing", reviews)
			}
		})
	}
}

func TestClaimReviewCheckpointProjectionDoesNotFinalizeLiveLedger(t *testing.T) {
	existing := &ClaimVerification{Status: "disagreed", Verdict: "false", ObservationIDs: []string{"fresh-1"}}
	result := AuditResult{CoverageNotes: []string{"original"}, Investigations: []Investigation{{ID: "a", Claim: "resolved", Status: "rejected"}, {ID: "b", Claim: "open", Status: "investigating"}, {ID: "c", Claim: "reviewed", Status: "supported", ClaimVerification: existing}}}
	for _, enabled := range []bool{false, true} {
		calls := 0
		sentinel := errors.New("checkpoint stopped")
		progress := claimReviewProgress(func(saved AuditResult, _ []ToolTrace) error {
			calls++
			status, notes := "disabled", 1
			if enabled {
				status, notes = "unavailable", 2
			}
			if saved.Investigations[0].ClaimVerification.Status != status || saved.Investigations[0].ClaimVerification.Verdict != "" || saved.Investigations[1].ClaimVerification != nil || saved.Investigations[2].ClaimVerification != existing || len(saved.CoverageNotes) != notes {
				t.Fatal("wrong checkpoint state", saved)
			}
			return sentinel
		}, Snapshot{BaseSHA: "base", HeadSHA: "head"}, enabled)
		if !errors.Is(progress(result, nil), sentinel) || calls != 1 {
			t.Fatal("checkpoint failure swallowed")
		}
		if result.Investigations[0].ClaimVerification != nil || len(result.CoverageNotes) != 1 {
			t.Fatal("live primary finalized")
		}
	}
	if claimReviewProgress(nil, Snapshot{}, true) != nil {
		t.Fatal("invented progress callback")
	}
}
