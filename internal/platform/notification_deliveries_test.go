package platform

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestNotificationQueueDeduplicationLeaseAndUnknownRecovery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	rev := int64(0)
	input := IntegrationInput{Integration: Integration{Name: "queue", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &rev, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example/events"}}
	integration, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil {
		t.Fatal(err)
	}
	summary := NotificationSummary{EventID: "unique-event", ProjectID: 1, Text: "summary"}
	id, err := s.QueueNotification(ctx, integration.ID, 1, summary)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.QueueNotification(ctx, integration.ID, 1, summary)
	if err != nil || id != again {
		t.Fatal(again, err)
	}
	d, err := s.ClaimNotification(ctx)
	if err != nil || d.ID != id || d.Attempt != 1 {
		t.Fatal(d, err)
	}
	if _, err = s.ClaimNotification(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("double claim", err)
	}
	wrong := d
	wrong.LeaseToken = "wrong"
	if err = s.FinishNotification(ctx, wrong, "delivered", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("fence missing", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET lease_until='2000-01-01T00:00:00Z' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimNotification(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	var state string
	if err = s.DB.QueryRow(`SELECT status FROM platform_notification_deliveries WHERE id=?`, id).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	if err = s.FinishNotification(ctx, d, "delivered", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("stale sender accepted", err)
	}
	summary.EventID = "second-event"
	id, err = s.QueueNotification(ctx, integration.ID, 1, summary)
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.ClaimNotification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rev = integration.Revision
	input.Credentials = nil
	input.Enabled = false
	if _, err = s.SaveIntegration(ctx, integration.ID, 1, input); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishNotification(ctx, d, "delivered", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("changed recipient accepted", err)
	}
	if err = s.DB.QueryRow(`SELECT status FROM platform_notification_deliveries WHERE id=?`, id).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
}
