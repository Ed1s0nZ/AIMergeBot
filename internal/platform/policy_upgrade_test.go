package platform

import (
	"context"
	"testing"
)

func TestPreviousReleasePolicyCannotAutomaticallyRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{Model: "frozen"}}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET policy_version=? WHERE id=?`, "eino-audit-contract-v12", id); err != nil {
		t.Fatal(err)
	}
	child, err := s.FailAndRetry(ctx, id, "temporary", AuditResult{Summary: "old evidence"}, nil, 0)
	if err != nil || child != 0 {
		t.Fatalf("old strategy retried: %d %v", child, err)
	}
	old, err := s.Run(ctx, id)
	if err != nil || old.RetryInfo == nil || old.RetryInfo.State != "policy_changed" || old.Result.Summary != "old evidence" {
		t.Fatalf("old evidence or stopping reason lost: %+v %v", old, err)
	}
	fresh, created, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil || !created || fresh == id {
		t.Fatalf("new policy submission reused old run: %d %v %v", fresh, created, err)
	}
	current, err := s.Run(ctx, fresh)
	if err != nil || current.PolicyVersion != PolicyVersion {
		t.Fatal("new submission did not capture current policy", err)
	}
}

func TestPreviousReleasePendingRunRejectedBeforeAuditor(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "one", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{Model: "frozen"}}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET policy_version=? WHERE id=?`, "eino-audit-contract-v12", id); err != nil {
		t.Fatal(err)
	}
	a := &blockingAuditor{started: make(chan struct{}, 1)}
	runner := &Runner{Store: s, Repository: runRepo{}, Auditor: a, Workers: 1}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	waitStatus(t, s, id, "failed")
	old, err := s.Run(ctx, id)
	if err != nil || old.Error != "audit policy changed; submit a new audit" {
		t.Fatalf("unexpected old-policy outcome: %+v %v", old, err)
	}
	select {
	case <-a.started:
		t.Fatal("old policy reached auditor")
	default:
	}
}
