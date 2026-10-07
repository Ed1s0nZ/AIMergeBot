package platform

import (
	"context"
	"strings"
	"testing"
)

func TestBotAdmissionCanRollbackRunAndReplayReceiptTogether(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	for _, commit := range []bool{false, true} {
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO platform_bot_callback_receipts(integration_id,request_hash,expires_at) VALUES(?,?,?)`, channel.ID, strings.Repeat("c", 64), 1800000000); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		id, created, err := s.enqueueTx(ctx, tx, snap, 1, true, true)
		if err != nil || !created || id <= 0 {
			tx.Rollback()
			t.Fatal(id, created, err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"platform_runs", "platform_bot_callback_receipts"} {
			var count int
			want := 0
			if commit {
				want = 1
			}
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != want {
				t.Fatal("partial admission persisted", table, count, err)
			}
		}
		var count int
		want := 0
		if commit {
			want = 1
		}
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events WHERE action='run.created'`).Scan(&count); err != nil || count != want {
			t.Fatal("run event partial commit", count, err)
		}
	}
}
