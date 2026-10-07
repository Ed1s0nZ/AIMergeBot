package platform

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func notificationFindingFixture(t *testing.T, path string) (*Store, int64, string) {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	ctx := context.Background()
	if err = s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f:part:7","severity":"high"}],"coverage_notes":[]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	return s, run, head
}

func assertNotificationFindingEvidence(t *testing.T, s *Store, key, finding, head string, revision, owner int64) {
	t.Helper()
	var gotFinding, gotHead string
	var gotRevision, gotOwner int64
	err := s.DB.QueryRow(`SELECT f.finding_id,f.head_sha,f.disposition_revision,f.owner FROM platform_notification_event_findings f JOIN platform_notification_events e ON e.id=f.event_id WHERE e.event_key=?`, key).Scan(&gotFinding, &gotHead, &gotRevision, &gotOwner)
	if err != nil || gotFinding != finding || gotHead != head || gotRevision != revision || gotOwner != owner {
		t.Fatalf("evidence %q: %q %q %d %d %v", key, gotFinding, gotHead, gotRevision, gotOwner, err)
	}
}

func TestNotificationFindingReviewEvidenceIsImmutableAndAtomic(t *testing.T) {
	s, run, head := notificationFindingFixture(t, ":memory:")
	ctx := context.Background()
	zero := int64(0)
	review := Review{RunID: run, FindingID: "f:part:7", Status: "accepted", ExpectedRevision: &zero}
	if err := s.SaveReview(ctx, review); err != nil {
		t.Fatal(err)
	}
	first := fmt.Sprintf("review:%d:f:part:7:1", run)
	assertNotificationFindingEvidence(t, s, first, "f:part:7", head, 0, 0)
	d, err := s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &zero, Status: "open", Owner: 1, HeadSHA: head})
	if err != nil {
		t.Fatal(err)
	}
	review = reviewAtCurrentRevision(t, s, ctx, review)
	review.Status = "false_positive"
	if err = s.SaveReview(ctx, review); err != nil {
		t.Fatal(err)
	}
	second := fmt.Sprintf("review:%d:f:part:7:2", run)
	assertNotificationFindingEvidence(t, s, second, "f:part:7", head, d.Revision, 1)
	rev := d.Revision
	if _, err = s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &rev, Status: "open", Owner: 0, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	assertNotificationFindingEvidence(t, s, second, "f:part:7", head, 1, 1)
	if _, err = s.DB.Exec(`CREATE TRIGGER fail_event_evidence BEFORE INSERT ON platform_notification_event_findings BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	review = reviewAtCurrentRevision(t, s, ctx, review)
	review.Status = "accepted"
	if err = s.SaveReview(ctx, review); err == nil {
		t.Fatal("evidence failure did not roll back review")
	}
	var version, events, hist int
	if err = s.DB.QueryRow(`SELECT revision FROM platform_reviews WHERE run_id=?`, run).Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events WHERE run_id=? AND kind='finding.reviewed'`, run).Scan(&events); err != nil || events != 2 {
		t.Fatal(events, err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_review_history WHERE run_id=?`, run).Scan(&hist); err != nil || hist != 2 {
		t.Fatal(hist, err)
	}
}

func TestNotificationFindingExpiryEvidenceRollbackAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	s, run, head := notificationFindingFixture(t, path)
	ctx := context.Background()
	zero := int64(0)
	if _, err := s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &zero, Status: "accepted", Reason: "fixture", Owner: 1, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_event_evidence BEFORE INSERT ON platform_notification_event_findings BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(2 * time.Hour)
	if err := s.ExpireDispositions(ctx, at); err == nil {
		t.Fatal("evidence failure did not roll back expiry")
	}
	report, err := s.Disposition(ctx, run, 1, "f:part:7")
	if err != nil || report.Current.Revision != 1 || report.Current.Status != "accepted" || len(report.History) != 1 {
		t.Fatal(report, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events WHERE kind='risk.expired'`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if _, err = s.DB.Exec(`DROP TRIGGER fail_event_evidence`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.ExpireDispositions(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	key := fmt.Sprintf("expiry:%d:f:part:7:2", run)
	assertNotificationFindingEvidence(t, s, key, "f:part:7", head, 2, 1)
	rev := int64(2)
	if _, err = s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &rev, Status: "open", Owner: 0, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	assertNotificationFindingEvidence(t, reopened, key, "f:part:7", head, 2, 1)
	if err = reopened.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events WHERE kind='risk.expired'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestNotificationFindingMigrationDoesNotInferLegacyOwnership(t *testing.T) {
	s, run, head := notificationFindingFixture(t, ":memory:")
	ctx := context.Background()
	zero := int64(0)
	if _, err := s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &zero, Status: "open", Owner: 1, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_notification_events(run_id,kind,event_key,severity,created_at) VALUES(?,'risk.expired','legacy:opaque',4,?)`, run, now()); err != nil {
		t.Fatal(err)
	}
	// Reconstruct the previous schema and old trigger definitions without guessing owners.
	for _, name := range []string{"platform_notification_review_insert", "platform_notification_review_update"} {
		if _, err := s.DB.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`DROP TABLE platform_notification_event_findings`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER platform_notification_review_insert AFTER INSERT ON platform_reviews BEGIN INSERT OR IGNORE INTO platform_notification_events(run_id,kind,event_key,severity,created_at) VALUES(NEW.run_id,'finding.reviewed','review:'||NEW.run_id||':'||NEW.finding_id||':'||NEW.revision,4,NEW.updated_at); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReview(ctx, Review{RunID: run, FindingID: "f:part:7", Status: "accepted", ExpectedRevision: &zero}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrate(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_event_findings`).Scan(&count); err != nil || count != 0 {
		t.Fatal("legacy ownership invented", count, err)
	}
	one := int64(1)
	if err := s.SaveReview(ctx, Review{RunID: run, FindingID: "f:part:7", Status: "false_positive", ExpectedRevision: &one}); err != nil {
		t.Fatal(err)
	}
	assertNotificationFindingEvidence(t, s, fmt.Sprintf("review:%d:f:part:7:2", run), "f:part:7", head, 1, 1)
}
