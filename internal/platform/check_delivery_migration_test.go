package platform

import (
	"context"
	"strings"
	"testing"
)

func TestCheckDeliveryLegacyMigrationPreservesExplicitAuthorization(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueRunCheck(ctx, id, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`ALTER TABLE platform_check_deliveries DROP COLUMN automatic`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateCheckDeliveries(tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimRunCheck(ctx)
	if err != nil || d.RunID != id || d.Automatic || d.Actor != 1 {
		t.Fatal("legacy authorization reinterpreted", d, err)
	}
}
