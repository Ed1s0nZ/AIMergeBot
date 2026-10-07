package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNotificationRetryAvailabilityCurrentConfiguration(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	rev := int64(0)
	input := IntegrationInput{Integration: Integration{Name: "fixture", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &rev, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example/PRIVATE-ENDPOINT"}}
	integration, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.QueueNotification(ctx, integration.ID, 1, NotificationSummary{EventID: "availability", ProjectID: 1, Text: "PRIVATE-PAYLOAD"})
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := s.ClaimNotification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishNotification(ctx, delivery, "failed", "provider_http_rejected"); err != nil {
		t.Fatal(err)
	}
	check := func(can bool, reason string) {
		t.Helper()
		records, total, err := s.NotificationRecords(ctx, 1, 1, 1)
		if err != nil || total != 1 || len(records) != 1 {
			t.Fatalf("records=%v total=%d err=%v", records, total, err)
		}
		if records[0].ID != id || records[0].CanRetry != can || records[0].RetryUnavailableReason != reason {
			t.Fatalf("availability=%+v", records[0])
		}
		body, err := json.Marshal(records)
		if err != nil || strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "lease_token") || strings.Contains(string(body), "credentials") || !strings.Contains(string(body), `"can_retry":`) {
			t.Fatalf("unsafe record JSON: %s err=%v", body, err)
		}
		var attempt int
		var status string
		if err = s.DB.QueryRow(`SELECT status,attempt FROM platform_notification_deliveries WHERE id=?`, id).Scan(&status, &attempt); err != nil || attempt != records[0].Attempt || status != records[0].Status {
			t.Fatal("list changed delivery", err)
		}
	}
	check(true, "")
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET status='unknown' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	check(true, "")
	// A real configuration save changes the captured/current revision relationship.
	input.Integration = integration
	input.Name = "renamed"
	rev = integration.Revision
	input.Credentials = nil
	changed, err := s.SaveIntegration(ctx, integration.ID, 1, input)
	if err != nil || changed.Revision <= rev {
		t.Fatal(changed, err)
	}
	check(false, "configuration_changed")
	attempt := 1
	if err = s.RetryNotification(ctx, id, 1, DeliveryRetryRequest{ExpectedAttempt: &attempt, ExpectedStatus: "unknown", AcknowledgeDuplicate: true}); err != ErrConflict {
		t.Fatal("stale configuration retry", err)
	}
	// Controlled database fixtures cover unavailable states without delivering messages.
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET integration_revision=? WHERE id=?`, changed.Revision, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_integrations SET enabled=0 WHERE id=?`, integration.ID); err != nil {
		t.Fatal(err)
	}
	check(false, "integration_disabled")
	for _, c := range []struct {
		status  string
		attempt int
		reason  string
	}{
		{"unknown", 5, "attempts_exhausted"}, {"failed", 0, "invalid_attempt"},
		{"accepted", 1, "state_not_retryable"}, {"delivered", 1, "state_not_retryable"},
		{"cancelled", 1, "state_not_retryable"}, {"pending", 0, "state_not_retryable"},
	} {
		if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET status=?,attempt=? WHERE id=?`, c.status, c.attempt, id); err != nil {
			t.Fatal(err)
		}
		check(false, c.reason)
	}
	// Model a damaged historical reference only in this temporary single-connection store.
	if _, err = s.DB.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET status='failed',attempt=1,integration_id=999999 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	check(false, "configuration_unavailable")
	empty, total, err := s.NotificationRecords(ctx, 1, 2, 1)
	if err != nil || total != 1 || len(empty) != 0 {
		t.Fatal("pagination", empty, total, err)
	}
	if _, _, err = s.NotificationRecords(ctx, 99999, 1, 1); err == nil {
		t.Fatal("non-admin could read availability")
	}
}
