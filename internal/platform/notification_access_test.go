package platform

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDigestAccessRevocationAndLegacyReceipt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "sender", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2, 3} {
		if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(?,'fixture',1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,'viewer')`, id, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	revision := int64(0)
	v, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "digest", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Events: []string{"run.completed"}, Frequency: "daily"}, ExpectedRevision: &revision, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example"}})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(time.Second)
	for i := 0; i < 2; i++ {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: i + 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, u.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,3,'context')`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded',audit_policy_json='{"context_repositories":[{"project_id":3,"sha":"cccccccccccccccccccccccccccccccccccccccc"}]}',finished_at=? WHERE id=?`, at.UTC().Format(time.RFC3339Nano), id); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.CollectNotifications(ctx, "", at.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	records, total, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 1 {
		t.Fatalf("%+v %d %v", records, total, err)
	}
	d := records[0]
	var refs int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_delivery_runs WHERE delivery_id=?`, d.ID).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("refs=%d %v", refs, err)
	}
	if d.IntegrationID != v.ID || s.requireNotificationAccess(ctx, d) != nil {
		t.Fatal("authorized digest rejected")
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=3 AND user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if s.requireNotificationAccess(ctx, d) == nil {
		t.Fatal("revoked context access accepted")
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET next_attempt=? WHERE id=?`, now(), d.ID); err != nil {
		t.Fatal(err)
	}
	sent, err := s.DispatchNotification(ctx)
	if err != nil || !sent {
		t.Fatal(sent, err)
	}
	var status, code string
	if err = s.DB.QueryRow(`SELECT status,error_code FROM platform_notification_deliveries WHERE id=?`, d.ID).Scan(&status, &code); err != nil || status != "cancelled" || code != "permission_changed" {
		t.Fatal(status, code, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_notification_delivery_runs WHERE delivery_id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if s.requireNotificationAccess(ctx, d) == nil {
		t.Fatal("legacy digest without identities accepted")
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET status='pending' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	var source int64
	if err = s.DB.QueryRow(`SELECT MIN(id) FROM platform_runs`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	summary := NotificationSummary{Version: "aimangebot.notification.v1", ProjectID: 1, EventID: d.EventKey, Text: "new digest\nNEW ONLY"}
	if err = appendDigestOutbox(ctx, tx, v, summary, at, source); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var newPayload string
	if err = s.DB.QueryRow(`SELECT payload FROM platform_notification_deliveries WHERE id!=? AND integration_id=?`, d.ID, v.ID).Scan(&newPayload); err != nil || !strings.Contains(newPayload, "NEW ONLY") || strings.Contains(newPayload, "状态") {
		t.Fatal("legacy content carried into verified digest", newPayload, err)
	}
	if err = s.DB.QueryRow(`SELECT status FROM platform_notification_deliveries WHERE id=?`, d.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatal("legacy pending remained active", status, err)
	}
	if err = s.requireNotificationAccess(ctx, NotificationDelivery{IntegrationID: v.ID, IntegrationRevision: v.Revision, ProjectID: 1, EventKey: "test:explicit"}); err != nil {
		t.Fatal("runless test incorrectly blocked", err)
	}
}
