package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNotificationRecordsRedactionAndExplicitUnknownRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	rev := int64(0)
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "fixture", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &rev, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example/secret"}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.QueueNotification(ctx, integration.ID, 1, NotificationSummary{EventID: "event", ProjectID: 1, Text: "PRIVATE-PAYLOAD"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimNotification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishNotification(ctx, d, "unknown", "transport_unknown"); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.NotificationRecords(ctx, 1, 1, 1)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatal(items, total, err)
	}
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "lease_token") {
		t.Fatal("private payload exposed")
	}
	attempt := 1
	req := DeliveryRetryRequest{ExpectedAttempt: &attempt, ExpectedStatus: "unknown"}
	if err = s.RetryNotification(ctx, id, 1, req); err != ErrIntegrationInput {
		t.Fatal("unknown retried without acknowledgement", err)
	}
	req.AcknowledgeDuplicate = true
	if err = s.RetryNotification(ctx, id, 1, req); err != nil {
		t.Fatal(err)
	}
	if err = s.RetryNotification(ctx, id, 1, req); err != ErrConflict {
		t.Fatal("double retry", err)
	}
	d, err = s.ClaimNotification(ctx)
	if err != nil || d.Attempt != 2 {
		t.Fatal(d, err)
	}
	if err = s.FinishNotification(ctx, d, "accepted", ""); err != nil {
		t.Fatal(err)
	}
	attempt = 2
	req.ExpectedStatus = "accepted"
	if err = s.RetryNotification(ctx, id, 1, req); err != ErrConflict {
		t.Fatal("accepted message resent", err)
	}
}
