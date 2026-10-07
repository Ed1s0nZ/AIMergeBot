package platform

import (
	"context"
	"errors"
	"testing"
)

// Existing fixtures explicitly read the current version, like a client refresh.
func reviewAtCurrentRevision(t *testing.T, s *Store, ctx context.Context, r Review) Review {
	t.Helper()
	reviews, err := s.Reviews(ctx, r.RunID)
	if err != nil {
		t.Fatal(err)
	}
	version := int64(0)
	for _, item := range reviews {
		if item.FindingID == r.FindingID {
			version = item.Revision
		}
	}
	r.ExpectedRevision = &version
	return r
}
func revisionFixture(t *testing.T) (*Store, int64) {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f"}}}, nil); err != nil {
		t.Fatal(err)
	}
	return s, id
}
func TestReviewRevisionConflictDoesNotMutateDecisionOrDelivery(t *testing.T) {
	s, id := revisionFixture(t)
	ctx := context.Background()
	zero := int64(0)
	r := Review{RunID: id, FindingID: "f", Status: "accepted", ExpectedRevision: &zero}
	if err := s.SaveReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	var generation int
	if err := s.DB.QueryRow(`SELECT desired_generation FROM platform_comment_delivery WHERE run_id=?`, id).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	r.Status = "false_positive"
	if err := s.SaveReview(ctx, r); !errors.Is(err, ErrReviewConflict) {
		t.Fatalf("stale first write: %v", err)
	}
	got, err := s.Reviews(ctx, id)
	if err != nil || len(got) != 1 || got[0].Revision != 1 || got[0].Status != "accepted" {
		t.Fatalf("stale write changed decision: %+v %v", got, err)
	}
	var after int
	s.DB.QueryRow(`SELECT desired_generation FROM platform_comment_delivery WHERE run_id=?`, id).Scan(&after)
	if after != generation {
		t.Fatal("conflict scheduled comment")
	}
	one := int64(1)
	r.ExpectedRevision = &one
	if err := s.SaveReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReview(ctx, r); !errors.Is(err, ErrReviewConflict) {
		t.Fatal("stale update succeeded")
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	got, err = s.Reviews(ctx, id)
	if err != nil || got[0].Revision != 2 || got[0].Status != "false_positive" {
		t.Fatal("migration changed decision")
	}
	r.ExpectedRevision = nil
	if err := s.SaveReview(ctx, r); !errors.Is(err, ErrReviewRevision) {
		t.Fatal("unconditional write accepted")
	}
}
func TestReviewRevisionConcurrentFirstSave(t *testing.T) {
	s, id := revisionFixture(t)
	zero := int64(0)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, status := range []string{"accepted", "false_positive"} {
		go func(status string) {
			<-start
			results <- s.SaveReview(context.Background(), Review{RunID: id, FindingID: "f", Status: status, ExpectedRevision: &zero})
		}(status)
	}
	close(start)
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, ErrReviewConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestReviewRevisionMigratesLegacyDecisionWithoutChangingHistory(t *testing.T) {
	s, id := revisionFixture(t)
	ctx := context.Background()
	r := reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "accepted", Reason: "legacy decision"})
	if err := s.SaveReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_review_history WHERE run_id=?`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// A pre-revision database never had the notification triggers. Remove
	// dependent modern schema before reconstructing that legacy fixture.
	for _, name := range []string{"platform_notification_review_insert", "platform_notification_review_update"} {
		if _, err := s.DB.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`ALTER TABLE platform_reviews DROP COLUMN revision`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	var notificationTriggers int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name IN ('platform_notification_review_insert','platform_notification_review_update')`).Scan(&notificationTriggers); err != nil || notificationTriggers != 2 {
		t.Fatalf("notification triggers not restored: %d %v", notificationTriggers, err)
	}
	var migrationEvents int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events WHERE kind='finding.reviewed' AND run_id=?`, id).Scan(&migrationEvents); err != nil || migrationEvents != 1 {
		t.Fatalf("migration changed notification history: %d %v", migrationEvents, err)
	}
	got, err := s.Reviews(ctx, id)
	if err != nil || len(got) != 1 || got[0].Revision != 1 || got[0].Reason != "legacy decision" {
		t.Fatalf("legacy migration lost decision: %+v %v", got, err)
	}
	var after int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_review_history WHERE run_id=?`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("migration duplicated history")
	}
}
