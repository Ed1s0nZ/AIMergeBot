package platform

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func notificationOwnerFixture(t *testing.T, frequency string) (*Store, int64, string, User, Integration) {
	t.Helper()
	s, run, head := notificationFindingFixture(t, filepath.Join(t.TempDir(), "owners.db"))
	ctx := context.Background()
	owner, err := s.CreateUser(ctx, "owner", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{2, 3} {
		if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(?,'fixture',1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{1, 2, 3} {
		if err = s.SetProjectMember(ctx, id, owner.ID, "viewer", 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET source_project_id=2,audit_policy_json='{"context_repositories":[{"project_id":3,"sha":"cccccccccccccccccccccccccccccccccccccccc"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	v, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "owner route", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, OwnerIDs: []int64{owner.ID}, Events: []string{"finding.reviewed", "risk.expired"}, Frequency: frequency}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &zero, Status: "open", Owner: owner.ID, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	return s, run, head, owner, v
}

func ownerReviewAndCollect(t *testing.T, s *Store, run int64) []NotificationDelivery {
	t.Helper()
	ctx := context.Background()
	r := reviewAtCurrentRevision(t, s, ctx, Review{RunID: run, FindingID: "f:part:7", Status: "accepted"})
	if err := s.SaveReview(ctx, r); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(10 * 24 * time.Hour)
	for i := 0; i < 2; i++ {
		if err := s.CollectNotifications(ctx, "https://audit.example", at); err != nil {
			t.Fatal(err)
		}
	}
	d, _, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNotificationOwnerRouteConfigPreservesOmissionAndCanClear(t *testing.T) {
	s, _, _, owner, v := notificationOwnerFixture(t, "instant")
	ctx := context.Background()
	rev := v.Revision
	input := IntegrationInput{Integration: v, ExpectedRevision: &rev}
	input.OwnerIDs = nil
	saved, err := s.SaveIntegration(ctx, v.ID, 1, input)
	if err != nil || len(saved.OwnerIDs) != 1 || saved.OwnerIDs[0] != owner.ID {
		t.Fatal(saved, err)
	}
	// Simulate a process restart, retaining both the route and its integration revision.
	var seq int
	var schema, path string
	if err = s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &schema, &path); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	list, err := reopened.Integrations(ctx, 1)
	if err != nil || len(list) != 1 || list[0].Revision != saved.Revision || len(list[0].OwnerIDs) != 1 {
		t.Fatal(list, err)
	}
	rev = saved.Revision
	input.ExpectedRevision = &rev
	input.OwnerIDs = []int64{}
	cleared, err := reopened.SaveIntegration(ctx, v.ID, 1, input)
	if err != nil || len(cleared.OwnerIDs) != 0 {
		t.Fatal(cleared, err)
	}
}

func TestNotificationOwnerRouteRejectsInvalidScopeAndRollback(t *testing.T) {
	s, _, _, owner, v := notificationOwnerFixture(t, "instant")
	ctx := context.Background()
	rev := v.Revision
	for _, ids := range [][]int64{{-1}, {0}, {owner.ID, owner.ID}, make([]int64, 101)} {
		in := IntegrationInput{Integration: v, ExpectedRevision: &rev}
		in.OwnerIDs = ids
		if _, err := s.SaveIntegration(ctx, v.ID, 1, in); !errors.Is(err, ErrIntegrationInput) {
			t.Fatal(ids, err)
		}
	}
	for _, kind := range []string{"github", "jira", "linear", "gitlab"} {
		in := v
		in.Kind = kind
		if validateNotificationOwnerRoute(in) == nil {
			t.Fatal(kind)
		}
	}
	for _, events := range [][]string{nil, {"run.completed"}, {"risk.expired", "run.failed"}} {
		in := v
		in.Events = events
		if validateNotificationOwnerRoute(in) == nil {
			t.Fatal(events)
		}
	}
	if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=1 AND user_id=?`, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveIntegration(ctx, v.ID, 1, IntegrationInput{Integration: v, ExpectedRevision: &rev}); err == nil {
		t.Fatal("revoked owner saved")
	}
	if err := s.SetProjectMember(ctx, 1, owner.ID, "viewer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_route BEFORE UPDATE ON platform_integration_owner_routes BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	in := IntegrationInput{Integration: v, ExpectedRevision: &rev}
	in.Name = "changed"
	in.OwnerIDs = []int64{1}
	if _, err := s.SaveIntegration(ctx, v.ID, 1, in); err == nil {
		t.Fatal("route write failure ignored")
	}
	list, err := s.Integrations(ctx, 1)
	if err != nil || list[0].Name != v.Name || list[0].Revision != rev || list[0].OwnerIDs[0] != owner.ID {
		t.Fatal(list, err)
	}
}

func TestNotificationOwnerRouteInstantDailyWeeklyAndRevocation(t *testing.T) {
	for _, freq := range []string{"instant", "daily", "weekly"} {
		t.Run(freq, func(t *testing.T) {
			s, run, _, owner, _ := notificationOwnerFixture(t, freq)
			ctx := context.Background()
			ds := ownerReviewAndCollect(t, s, run)
			if len(ds) != 1 {
				t.Fatal(ds)
			}
			d := ds[0]
			if err := s.requireNotificationAccess(ctx, d); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_delivery_events WHERE delivery_id=?`, d.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal(count, err)
			}
			var payload string
			if err := s.DB.QueryRow(`SELECT payload FROM platform_notification_deliveries WHERE id=?`, d.ID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(payload, "sourceEvent") || strings.Contains(payload, "owner_ids") {
				t.Fatal("internal identities leaked", payload)
			}
			if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=3 AND user_id=?`, owner.ID); err != nil {
				t.Fatal(err)
			}
			if s.requireNotificationAccess(ctx, d) == nil {
				t.Fatal("context revocation ignored")
			}
			if _, err := s.DB.Exec(`UPDATE platform_notification_deliveries SET next_attempt=? WHERE id=?`, now(), d.ID); err != nil {
				t.Fatal(err)
			}
			sent, err := s.DispatchNotification(ctx)
			if !sent || err != nil {
				t.Fatal(sent, err)
			}
			var status, code string
			if err = s.DB.QueryRow(`SELECT status,error_code FROM platform_notification_deliveries WHERE id=?`, d.ID).Scan(&status, &code); err != nil || status != "cancelled" || code != "permission_changed" {
				t.Fatal(status, code, err)
			}
		})
	}
}

func TestNotificationOwnerRouteRejectsChangedAssignmentAndMissingEvidence(t *testing.T) {
	for _, mutation := range []string{"owner", "revision", "head", "disabled", "source", "target", "project", "proof"} {
		t.Run(mutation, func(t *testing.T) {
			s, run, head, owner, _ := notificationOwnerFixture(t, "instant")
			ctx := context.Background()
			ds := ownerReviewAndCollect(t, s, run)
			if len(ds) != 1 {
				t.Fatal(ds)
			}
			d := ds[0]
			var err error
			switch mutation {
			case "owner":
				rev := int64(1)
				_, err = s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &rev, Status: "open", Owner: 1, HeadSHA: head})
			case "revision":
				rev := int64(1)
				_, err = s.SaveDisposition(ctx, run, 1, "f:part:7", DispositionRequest{ExpectedRevision: &rev, Status: "open", Owner: owner.ID, HeadSHA: head})
			case "head":
				_, err = s.DB.Exec(`UPDATE platform_runs SET head_sha=? WHERE id=?`, strings.Repeat("d", 40), run)
			case "disabled":
				_, err = s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, owner.ID)
			case "source":
				_, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=2 AND user_id=?`, owner.ID)
			case "target":
				_, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=1 AND user_id=?`, owner.ID)
			case "project":
				_, err = s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=3`)
			case "proof":
				_, err = s.DB.Exec(`DELETE FROM platform_notification_delivery_events WHERE delivery_id=?`, d.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.requireNotificationAccess(ctx, d) == nil {
				t.Fatal("changed evidence accepted", mutation)
			}
		})
	}
}

func TestNotificationOwnerRouteFiltersLegacyUnassignedAndManualBypass(t *testing.T) {
	s, run, _, _, v := notificationOwnerFixture(t, "instant")
	ctx := context.Background()
	if _, err := s.DB.Exec(`INSERT INTO platform_notification_events(run_id,kind,event_key,created_at) VALUES(?,'risk.expired','legacy',?)`, run, now()); err != nil {
		t.Fatal(err)
	}
	if err := s.CollectNotifications(ctx, "", time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ds, total, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 0 {
		t.Fatal(ds, total, err)
	}
	for _, sum := range []NotificationSummary{{ProjectID: 1, RunID: run, EventID: "test-bypass", Text: "test"}, {ProjectID: 1, EventID: "manual", Text: "test"}} {
		if _, err = s.QueueNotification(ctx, v.ID, 1, sum); !errors.Is(err, ErrIntegrationInput) {
			t.Fatal(err)
		}
	}
	id, err := s.QueueNotification(ctx, v.ID, 1, NotificationSummary{ProjectID: 1, EventID: "test-explicit", Text: "fixed test"})
	if err != nil {
		t.Fatal(err)
	}
	ds, total, err = s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 1 || ds[0].ID != id || s.requireNotificationAccess(ctx, ds[0]) != nil {
		t.Fatal(ds, total, err)
	}
	rev := v.Revision
	if _, err = s.SaveIntegration(ctx, v.ID, 1, IntegrationInput{Integration: v, ExpectedRevision: &rev}); err != nil {
		t.Fatal(err)
	}
	if s.requireNotificationAccess(ctx, ds[0]) == nil {
		t.Fatal("explicit test bypassed changed channel version")
	}
	// JSON never persists the internal event association inside the externally sent payload.
	b, err := json.Marshal(NotificationSummary{EventID: "test", sourceEventID: 123})
	if err != nil || strings.Contains(string(b), "123") {
		t.Fatal(string(b), err)
	}
}

func TestNotificationOwnerDigestSplitsBoundedEvidenceAndRejectsPartialLoss(t *testing.T) {
	s, run, _, _, _ := notificationOwnerFixture(t, "daily")
	ctx := context.Background()
	zero := int64(0)
	if err := s.SaveReview(ctx, Review{RunID: run, FindingID: "f:part:7", Status: "accepted", ExpectedRevision: &zero}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 201; i++ {
		if _, err := s.DB.Exec(`UPDATE platform_reviews SET revision=revision+1 WHERE run_id=? AND finding_id='f:part:7'`, run); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now().Add(10 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		if err := s.CollectNotifications(ctx, "", at); err != nil {
			t.Fatal(err)
		}
	}
	ds, total, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 2 {
		t.Fatal(ds, total, err)
	}
	sum := 0
	var large NotificationDelivery
	for _, d := range ds {
		var count int
		if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_delivery_events WHERE delivery_id=?`, d.ID).Scan(&count); err != nil || count < 1 || count > 200 {
			t.Fatal(count, err)
		}
		sum += count
		if count == 200 {
			large = d
		}
		if err = s.requireNotificationAccess(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if sum != 202 || large.ID == 0 {
		t.Fatal(sum, large)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_notification_delivery_events WHERE delivery_id=? AND event_id=(SELECT MIN(event_id) FROM platform_notification_delivery_events WHERE delivery_id=?)`, large.ID, large.ID); err != nil {
		t.Fatal(err)
	}
	if s.requireNotificationAccess(ctx, large) == nil {
		t.Fatal("partially missing source evidence accepted")
	}
}

func TestNotificationOwnerOutboxEvidenceFailureRollsBackRouting(t *testing.T) {
	s, run, _, _, _ := notificationOwnerFixture(t, "instant")
	ctx := context.Background()
	zero := int64(0)
	if err := s.SaveReview(ctx, Review{RunID: run, FindingID: "f:part:7", Status: "accepted", ExpectedRevision: &zero}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_delivery_evidence BEFORE INSERT ON platform_notification_delivery_events BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.CollectNotifications(ctx, "", time.Now().Add(time.Second)); err == nil {
		t.Fatal("outbox source failure ignored")
	}
	for _, table := range []string{"platform_notification_deliveries", "platform_notification_event_routes", "platform_notification_delivery_event_counts"} {
		var count int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if _, err := s.DB.Exec(`DROP TRIGGER fail_delivery_evidence`); err != nil {
		t.Fatal(err)
	}
	if err := s.CollectNotifications(ctx, "", time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ds, total, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 1 || s.requireNotificationAccess(ctx, ds[0]) != nil {
		t.Fatal(ds, total, err)
	}
}

func TestNotificationOwnerRouteDeliveryEvidenceSurvivesRestart(t *testing.T) {
	s, run, _, _, _ := notificationOwnerFixture(t, "daily")
	ctx := context.Background()
	ds := ownerReviewAndCollect(t, s, run)
	if len(ds) != 1 {
		t.Fatal(ds)
	}
	var seq int
	var name, path string
	if err := s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.requireNotificationAccess(ctx, ds[0]); err != nil {
		t.Fatal(err)
	}
	if err = reopened.CollectNotifications(ctx, "", time.Now().Add(10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, total, err := reopened.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 1 || after[0].ID != ds[0].ID {
		t.Fatal(after, total, err)
	}
}
