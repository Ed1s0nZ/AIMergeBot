package platform

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCollectorBoundedFairRescanAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checks.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	ctx := context.Background()
	if err = s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	policy, err := s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{Checks: &CheckRules{Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	var damaged int64
	for mr := 1; mr <= 106; mr++ {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: &AuditPolicy{Workflow: &policy}}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		if mr == 1 {
			damaged = id
		}
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json=json_set(audit_policy_json,'$.workflow.focus','invalid') WHERE id=?`, damaged); err != nil {
		t.Fatal(err)
	}
	count, err := s.CollectRunChecks(ctx)
	if count != 99 || err == nil {
		t.Fatal("bounded batch", count, err)
	}
	count, err = s.CollectRunChecks(ctx)
	if count != 6 || err == nil {
		t.Fatal("rejected row starved later work", count, err)
	}
	var total int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_check_deliveries`).Scan(&total); err != nil || total != 105 {
		t.Fatal(total, err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	count, err = s.CollectRunChecks(ctx)
	if count != 0 || err == nil {
		t.Fatal("restart duplicated delivery", count, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json=json_set(audit_policy_json,'$.workflow.focus',json('[]')) WHERE id=?`, damaged); err != nil {
		t.Fatal(err)
	}
	count, err = s.CollectRunChecks(ctx)
	if count != 1 || err != nil {
		t.Fatal("repaired record was lost", count, err)
	}
	count, err = s.CollectRunChecks(ctx)
	if count != 0 || err != nil {
		t.Fatal("repeated collection", count, err)
	}
}
