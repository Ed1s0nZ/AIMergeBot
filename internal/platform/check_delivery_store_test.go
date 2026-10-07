package platform

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestCheckDeliveryIdempotencyLatestRunAndLeaseFence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	create := func(head string) int64 {
		t.Helper()
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := create(strings.Repeat("b", 40))
	reader, err := s.CreateUser(ctx, "reader", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'operator')`, reader.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueRunCheck(ctx, first, reader.ID, false); err == nil {
		t.Fatal("operator queued admin-only publication")
	}

	for i := 0; i < 2; i++ {
		if err := s.QueueRunCheck(ctx, first, 1, false); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_check_deliveries`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := s.QueueRunCheck(ctx, first, 1, true); err != ErrConflict {
		t.Fatal("mode overwrite", err)
	}
	second := create(strings.Repeat("c", 40))
	if err := s.QueueRunCheck(ctx, first, 1, false); err != ErrConflict {
		t.Fatal("old run queued", err)
	}
	if err := s.QueueRunCheck(ctx, second, 1, false); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimRunCheck(ctx)
	if err != nil || d.RunID != second || d.Lease == "" {
		t.Fatal(d, err)
	}
	var state string
	if err = s.DB.QueryRow(`SELECT state FROM platform_check_deliveries WHERE run_id=?`, first).Scan(&state); err != nil || state != "stale" {
		t.Fatal(state, err)
	}
	if _, err = s.ClaimRunCheck(ctx); err != sql.ErrNoRows {
		t.Fatal("double claim", err)
	}
	stale := d
	stale.Lease = "other"
	if err = s.FinishRunCheck(ctx, stale, CheckPublication{State: "published", RemoteID: 42}); err != ErrConflict {
		t.Fatal("wrong lease accepted", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_check_deliveries SET lease_until='2000-01-01T00:00:00Z' WHERE run_id=?`, d.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimRunCheck(ctx); err != sql.ErrNoRows {
		t.Fatal("expired send replayed", err)
	}
	if err = s.FinishRunCheck(ctx, d, CheckPublication{State: "published", RemoteID: 42}); err != ErrConflict {
		t.Fatal("old ack accepted", err)
	}
	if err = s.DB.QueryRow(`SELECT state FROM platform_check_deliveries WHERE run_id=?`, second).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	if err = s.QueueRunCheck(ctx, second, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimRunCheck(ctx); err != sql.ErrNoRows {
		t.Fatal("idempotent enqueue replayed unknown", err)
	}
	third := create(strings.Repeat("d", 40))
	if err = s.QueueRunCheck(ctx, third, 1, false); err != nil {
		t.Fatal(err)
	}
	d, err = s.ClaimRunCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishRunCheck(ctx, d, CheckPublication{State: "published"}); err != ErrConflict {
		t.Fatal("empty receipt accepted", err)
	}
	if err = s.FinishRunCheck(ctx, d, CheckPublication{State: "published", RemoteID: 43}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishRunCheck(ctx, d, CheckPublication{State: "published", RemoteID: 43}); err != ErrConflict {
		t.Fatal("duplicate ack accepted", err)
	}
}
