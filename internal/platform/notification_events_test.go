package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNotificationEventsAtomicSeverityRoutingAndDigest(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	revision := int64(0)
	channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "events", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Events: []string{"run.completed"}, Frequency: "instant", MinimumSeverity: "high"}, ExpectedRevision: &revision, Credentials: &IntegrationCredentials{Endpoint: "https://receiver.example"}})
	if err != nil {
		t.Fatal(err)
	}
	create := func(i int, status string, finished time.Time) int64 {
		t.Helper()
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: i, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status=?,finished_at=?,result_json='{"findings":[{"id":"high","severity":"high"}],"coverage_notes":[]}' WHERE id=?`, status, finished.UTC().Format(time.RFC3339Nano), id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	at := time.Now().Add(time.Second)
	id := create(1, "succeeded", at)
	var severity int
	if err = s.DB.QueryRow(`SELECT severity FROM platform_notification_events WHERE run_id=?`, id).Scan(&severity); err != nil || severity != 4 {
		t.Fatal("trigger read stale projection", severity, err)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_runs SET status='failed' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events`).Scan(&count)
	if count != 1 {
		t.Fatal("rollback event leaked", count)
	}
	for i := 0; i < 2; i++ {
		if err = s.CollectNotifications(ctx, "https://audit.example", at.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	records, total, err := s.NotificationRecords(ctx, 1, 1, 20)
	if err != nil || total != 1 || records[0].IntegrationID != channel.ID {
		t.Fatal(records, total, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_events SET created_at=? WHERE run_id=?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
	revision = channel.Revision
	channel, err = s.SaveIntegration(ctx, channel.ID, 1, IntegrationInput{Integration: Integration{Name: "daily", Kind: "webhook", Enabled: true, ProjectIDs: []int{1}, Events: []string{"run.completed"}, Frequency: "daily"}, ExpectedRevision: &revision})
	if err != nil {
		t.Fatal(err)
	}
	current := time.Now().Add(2 * time.Second)
	create(2, "incomplete", current)
	create(3, "succeeded", current)
	future := current.Add(48 * time.Hour)
	for i := 0; i < 2; i++ {
		if err = s.CollectNotifications(ctx, "https://audit.example", future); err != nil {
			t.Fatal(err)
		}
	}
	var payload string
	if err = s.DB.QueryRow(`SELECT payload FROM platform_notification_deliveries WHERE integration_revision=? AND status='pending'`, channel.Revision).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var summary NotificationSummary
	if err = json.Unmarshal([]byte(payload), &summary); err != nil {
		t.Fatal(err)
	}
	if strings.Count(summary.Text, "状态") != 2 || summary.RunID != 0 || !strings.Contains(summary.URL, "tasks") {
		t.Fatal(summary)
	}
	if _, err = s.DB.Exec(`UPDATE platform_notification_deliveries SET status='accepted' WHERE integration_revision=?`, channel.Revision); err != nil {
		t.Fatal(err)
	}
	create(4, "succeeded", current.Add(time.Second))
	if err = s.CollectNotifications(ctx, "https://audit.example", future.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_deliveries WHERE integration_revision=?`, channel.Revision).Scan(&count)
	if count != 2 {
		t.Fatal("late event silently dropped", count, fmt.Sprint(channel))
	}
}
func TestDigestWindowUsesShanghaiMonday(t *testing.T) {
	at := time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)
	start, end := digestWindow("daily", at)
	if start.Format("2006-01-02") != "2026-10-06" || end.Format("2006-01-02") != "2026-10-07" {
		t.Fatal(start, end)
	}
	start, end = digestWindow("weekly", at)
	if start.Weekday() != time.Monday || start.Format("2006-01-02") != "2026-09-28" || end.Format("2006-01-02") != "2026-10-05" {
		t.Fatal(start, end)
	}
}
