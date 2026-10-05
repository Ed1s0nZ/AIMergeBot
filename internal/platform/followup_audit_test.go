package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type followupRepo struct {
	runRepo
	expected Snapshot
	changes  []Change
	onRead   func()
}

func (r followupRepo) Snapshot(context.Context, int, int) (Snapshot, error) {
	return Snapshot{}, errors.New("must not follow current MR")
}
func (r followupRepo) Changes(_ context.Context, s Snapshot) ([]Change, []string, error) {
	if s.BaseSHA != r.expected.BaseSHA || s.HeadSHA != r.expected.HeadSHA || s.DiffVersionID != r.expected.DiffVersionID {
		return nil, nil, errors.New("wrong pinned version")
	}
	if r.onRead != nil {
		r.onRead()
	}
	return r.changes, nil, nil
}
func followupFixture(t *testing.T) (*Store, *Runner, Run) {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, DiffVersionID: 7, BaseSHA: "base", HeadSHA: "original-head", AuditPolicy: &AuditPolicy{Model: "frozen-model", Excluded: []string{"md"}}}
	parent := lifecycleRun(t, s, snap, nil, "incomplete")
	repo := followupRepo{expected: snap, changes: []Change{{NewPath: "a.any", Diff: "@@ -0,0 +1 @@\n+first"}, {NewPath: "b.any", Diff: "@@ -0,0 +1 @@\n+second"}, {NewPath: "readme.md", Diff: "@@ -0,0 +1 @@\n+excluded"}}}
	return s, &Runner{Store: s, Repository: repo}, parent
}
func TestFollowupPinnedScopeDedupAndParentPreserved(t *testing.T) {
	s, r, parent := followupFixture(t)
	ctx := context.Background()
	scope, err := r.Scope(ctx, parent.ID, 1)
	if err != nil || len(scope.Files) != 3 || scope.Files[2].Selectable {
		t.Fatal("scope eligibility", scope, err)
	}
	id, created, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any", "a.any", "b.any"})
	if err != nil || !created {
		t.Fatal(err)
	}
	again, created, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"a.any", "b.any"})
	if err != nil || created || again != id {
		t.Fatal("duplicate follow-up", err)
	}
	child, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if child.BaseSHA != parent.BaseSHA || child.HeadSHA != parent.HeadSHA || child.DiffVersionID != 7 || child.AuditPolicy.FollowupOf != parent.ID || strings.Join(child.AuditPolicy.SelectedFiles, ",") != "a.any,b.any" || child.AuditPolicy.Model != "frozen-model" {
		t.Fatal("snapshot or policy changed")
	}
	original, err := s.Run(ctx, parent.ID)
	if err != nil || original.Status != "incomplete" || original.AuditPolicy.FollowupOf != 0 || len(original.AuditPolicy.SelectedFiles) != 0 {
		t.Fatal("parent changed")
	}
	for _, files := range [][]string{nil, {"readme.md"}, {"unknown.any"}, {"../escape"}, {"a.any\n"}} {
		if _, _, err = r.SubmitFollowup(ctx, parent.ID, 1, files); !errors.Is(err, ErrFollowupScope) {
			t.Fatalf("invalid scope accepted %v", err)
		}
	}
	// Mutable project state is rechecked after Git retrieval, inside admission.
	repo := r.Repository.(followupRepo)
	repo.onRead = func() {
		if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: false}); err != nil {
			t.Fatal(err)
		}
	}
	r.Repository = repo
	if _, _, err = r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"}); !errors.Is(err, ErrConflict) {
		t.Fatal("disabled while fetching admitted", err)
	}
}
func TestFollowupForkACLAndActiveParent(t *testing.T) {
	s, r, parent := followupFixture(t)
	ctx := context.Background()
	user, err := s.CreateUser(ctx, "member", "a-member-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 1, user.ID, "viewer", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.SubmitFollowup(ctx, parent.ID, user.ID, []string{"a.any"}); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer admitted", err)
	}
	if err = s.SaveProject(ctx, Project{ID: 2, Name: "fork", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET source_project_id=2 WHERE id=?`, parent.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 1, user.ID, "operator", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.SubmitFollowup(ctx, parent.ID, user.ID, []string{"a.any"}); err == nil {
		t.Fatal("fork permission bypass")
	}
	if err = s.SetProjectMember(ctx, 2, user.ID, "viewer", 1); err != nil {
		t.Fatal(err)
	}
	id, _, err := r.SubmitFollowup(ctx, parent.ID, user.ID, []string{"a.any"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.SubmitFollowup(ctx, id, 1, []string{"b.any"}); !errors.Is(err, ErrFollowupScope) {
		t.Fatal("active parent admitted", err)
	}
}

type followupCaptureAuditor struct{ scope chan DiffScope }

func (a followupCaptureAuditor) Audit(_ context.Context, _ Snapshot, scope DiffScope) (AuditResult, []ToolTrace, error) {
	a.scope <- scope
	return AuditResult{Findings: []Finding{}, Summary: "selected fixture", CoverageNotes: scope.Notes}, nil, nil
}
func TestFollowupWorkerFiltersAnchorScope(t *testing.T) {
	s, r, parent := followupFixture(t)
	ctx := context.Background()
	capture := followupCaptureAuditor{scope: make(chan DiffScope, 1)}
	r.Auditor = capture
	r.Workers = 1
	r.Timeout = 5 * time.Second
	id, _, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	select {
	case scope := <-capture.scope:
		if strings.Contains(scope.Text, "first") || !strings.Contains(scope.Text, "second") || scope.Added["a.any"] != nil || len(scope.Notes) == 0 {
			t.Fatal("wrong selected scope")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not audit")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := s.Run(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == "incomplete" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("followup claimed full coverage")
}

func TestFollowupAdmissionCannotForgeParentPolicy(t *testing.T) {
	s, r, parent := followupFixture(t)
	ctx := context.Background()
	snap := parent.Snapshot
	p := *parent.AuditPolicy
	p.FollowupOf = parent.ID
	p.SelectedFiles = []string{"b.any"}
	p.Model = "changed-model"
	snap.AuditPolicy = &p
	if _, _, err := s.EnqueueUser(ctx, snap, 1, true); !errors.Is(err, ErrFollowupScope) {
		t.Fatal("modified frozen policy admitted", err)
	}
	p.Model = parent.AuditPolicy.Model
	snap.HeadSHA = "other-head"
	if _, _, err := s.EnqueueUser(ctx, snap, 1, true); !errors.Is(err, ErrFollowupScope) {
		t.Fatal("modified snapshot admitted", err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='null' WHERE id=?`, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"}); !errors.Is(err, ErrFollowupScope) {
		t.Fatal("historical uncaptured task admitted", err)
	}
}

func TestFollowupSharesOutstandingQuota(t *testing.T) {
	s, r, parent := followupFixture(t)
	ctx := context.Background()
	settings, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.Snapshot()
	cfg.AuditQuotas.OutstandingGlobal = 1
	if err = settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s.BindQuotaSettings(settings)
	unrelated := parent.Snapshot
	unrelated.MRIID = 2
	id, _, err := s.EnqueueUser(ctx, unrelated, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"})
	var quota *QuotaError
	if !errors.As(err, &quota) {
		t.Fatal("follow-up bypassed quota", err)
	}
	if err = s.CancelUser(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	child, _, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"})
	if err != nil {
		t.Fatal(err)
	}
	again, created, err := r.SubmitFollowup(ctx, parent.ID, 1, []string{"b.any"})
	if err != nil || created || again != child {
		t.Fatal("duplicate charged quota", err)
	}
}
