package platform

import (
	"context"
	"strings"
	"testing"
)

func TestAutomaticChecksRequireCurrentCapturedAuthorization(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	policy, err := s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{Checks: &CheckRules{Enabled: true, Mode: "advisory"}})
	if err != nil {
		t.Fatal(err)
	}
	create := func(mr int, p *WorkflowPolicy) int64 {
		t.Helper()
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: &AuditPolicy{Workflow: p}}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	id := create(1, &policy)
	if err = s.QueueAutomaticRunCheck(ctx, id); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimRunCheck(ctx)
	if err != nil || !d.Automatic {
		t.Fatal(d, err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err != nil {
		t.Fatal(err)
	}
	forged := d
	forged.Automatic = false
	if _, err = s.CheckPublicationRun(ctx, forged); err != ErrConflict {
		t.Fatal("automatic flag bypassed", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_workflow_policies SET revision=2 WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err != ErrConflict {
		t.Fatal("superseded policy passed send preflight", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_workflow_policies SET revision=1 WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	missing := create(2, nil)
	if err = s.QueueAutomaticRunCheck(ctx, missing); err != ErrConflict {
		t.Fatal("uncaptured automatic authorization accepted", err)
	}
	old := create(3, &policy)
	policy.Checks.Enabled = false
	if _, err = s.SaveWorkflowPolicy(ctx, 1, 1, 1, policy); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueAutomaticRunCheck(ctx, old); err != ErrConflict {
		t.Fatal("disabled or superseded authorization accepted", err)
	}
	var state string
	if err = s.DB.QueryRow(`SELECT state FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state); err != nil || state != "unknown" {
		t.Fatal(state, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_check_deliveries WHERE run_id=?`, old).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid automatic queue persisted", count, err)
	}
}
