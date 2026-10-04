package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLifecycleProjectionAndReviewReceiptsAreTransactional(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	f := Finding{ID: "finding", File: "file.any", Side: "head", Type: "risk", Evidence: "danger(input)", Trigger: "untrusted input"}
	f.Fingerprint = findingFingerprint(snap, f)
	result := AuditResult{Findings: []Finding{f}, Summary: "synthetic source", CoverageNotes: []string{}}
	if err = s.Finish(ctx, id, "succeeded", "", result, nil); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"accepted", "fixed"} {
		if err = s.SaveReview(ctx, Review{RunID: id, FindingID: f.ID, Status: status, Reason: "human decision", Actor: 1}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_review_history WHERE run_id=?`, id).Scan(&count); err != nil || count != 2 {
		t.Fatal("review history overwritten", count, err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_reviews SET status='pending' WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_review_history WHERE run_id=?`, id).Scan(&count)
	if count != 2 {
		t.Fatal("receipt escaped transaction rollback")
	}
	var fingerprint string
	if err = s.DB.QueryRow(`SELECT fingerprint FROM platform_finding_occurrences WHERE run_id=?`, id).Scan(&fingerprint); err != nil || fingerprint != f.Fingerprint {
		t.Fatal("occurrence not synchronized", fingerprint, err)
	}
	raw, _ := json.Marshal(AuditResult{Findings: []Finding{}, Summary: "removed from canonical result", CoverageNotes: []string{}})
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json=? WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_finding_occurrences WHERE run_id=?`, id).Scan(&count)
	if count != 0 {
		t.Fatal("removed finding occurrence retained")
	}
	if err = s.SaveReview(ctx, Review{RunID: id, FindingID: f.ID, Status: "fixed", Actor: 1}); err == nil {
		t.Fatal("removed finding accepted review")
	}
}

func TestLifecycleBackfillPreservesHistoricalResultBytes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	f := Finding{ID: "old-id", File: "file.any", Side: "head", Type: "risk", Evidence: "danger(input)", Trigger: "untrusted input"}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{f}, Summary: "historic", CoverageNotes: []string{}}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveReview(ctx, Review{RunID: id, FindingID: f.ID, Status: "accepted", Actor: 1}); err != nil {
		t.Fatal(err)
	}
	var original, trace string
	s.DB.QueryRow(`SELECT result_json,trace_json FROM platform_runs WHERE id=?`, id).Scan(&original, &trace)
	for _, q := range []string{`DELETE FROM platform_projection_schema WHERE name='finding_lifecycle'`, `DELETE FROM platform_finding_occurrences`, `DELETE FROM platform_review_history`} {
		if _, err = s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	var actual, actualTrace, fingerprint string
	s.DB.QueryRow(`SELECT result_json,trace_json FROM platform_runs WHERE id=?`, id).Scan(&actual, &actualTrace)
	if original != actual || trace != actualTrace {
		t.Fatal("historical checkpoint rewritten")
	}
	if err = s.DB.QueryRow(`SELECT fingerprint FROM platform_finding_occurrences WHERE run_id=?`, id).Scan(&fingerprint); err != nil || fingerprint != findingFingerprint(snap, f) {
		t.Fatal("historical identity missing", fingerprint, err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	var count, imported int
	if err = s.DB.QueryRow(`SELECT COUNT(*),SUM(imported) FROM platform_review_history WHERE run_id=?`, id).Scan(&count, &imported); err != nil || count != 1 || imported != 1 {
		t.Fatal("baseline duplicated or fabricated", count, imported, err)
	}
}
