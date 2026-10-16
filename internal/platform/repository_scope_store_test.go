package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func injectRepositoryScope(t *testing.T, s *Store, id int64, policy *AuditPolicy) {
	t.Helper()
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json=? WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryScopeUnsupportedAdmissionAndExecution(t *testing.T) {
	v2 := repositoryScopeFixture()
	v2.Version = 2
	for name, scope := range map[string]*RepositoryScope{
		"v1_integration": repositoryScopeFixture(),
		"v2_integration": v2,
		"v2_legacy":      legacyScopeFixture(t),
	} {
		t.Run(name, func(t *testing.T) {
			testRepositoryScopeUnsupportedAdmissionAndExecution(t, scope)
		})
	}
}

func testRepositoryScopeUnsupportedAdmissionAndExecution(t *testing.T, scope *RepositoryScope) {
	t.Helper()
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	snap := repositoryScopeSnapshot(scope)
	var events int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnqueueUser(ctx, snap, admin.ID, false); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal(err)
	}
	var runs, after int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if runs != 0 || after != events {
		t.Fatal("rejected scope wrote run or event", runs, after, events)
	}
	policy := snap.AuditPolicy
	snap.AuditPolicy = nil
	id, created, err := s.EnqueueUser(ctx, snap, admin.ID, false)
	if err != nil || !created {
		t.Fatal("legacy admission", err)
	}
	// Model a stored future snapshot without bypassing the real execution path.
	injectRepositoryScope(t, s, id, policy)
	stored, err := snapshotForRun(ctx, s.DB, id)
	if err != nil || validateRepositoryScopeSnapshot(stored) != nil || stored.MRIID != snap.MRIID {
		t.Fatal("complete snapshot load", err)
	}
	repo := &bindingGuardRepository{}
	auditor := &blockingAuditor{started: make(chan struct{}, 1)}
	runner := &Runner{Store: s, Repository: repo, Auditor: auditor, Workers: 1, Timeout: time.Minute}
	if err = runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	waitStatus(t, s, id, "failed")
	if repo.snapshots.Load() != 0 || repo.changes.Load() != 0 || len(auditor.started) != 0 {
		t.Fatal("future scope reached legacy repository or model")
	}
	if err = runner.authorizeContextRead(ctx, Run{ID: id, Snapshot: stored, RequestedBy: admin.ID}); err == nil {
		t.Fatal("terminal scope authorized context")
	}
}

func TestRepositoryScopeStopsDurableRetryWithoutBinding(t *testing.T) {
	for _, mode := range []string{"valid", "null", "case_alias", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			s, admin, _ := accessFixture(t)
			ctx := context.Background()
			snap := repositoryScopeSnapshot(repositoryScopeFixture())
			policy := snap.AuditPolicy
			snap.AuditPolicy = nil
			id, _, err := s.EnqueueUser(ctx, snap, admin.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "valid":
				injectRepositoryScope(t, s, id, policy)
			case "null":
				_, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='{"repository_scope":null}' WHERE id=?`, id)
			case "case_alias":
				_, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='{"REPOSITORY_SCOPE":null}' WHERE id=?`, id)
			case "malformed":
				_, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='{' WHERE id=?`, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = requireLegacyRunRepository(ctx, s.DB, id); !errors.Is(err, ErrRepositoryUnavailable) {
				t.Fatal("durable guard accepted scope evidence", err)
			}
			child, err := s.FailAndRetry(ctx, id, "temporary", AuditResult{}, nil, 0)
			if child != 0 || (mode == "valid" && err != nil) || (mode != "valid" && err == nil) {
				t.Fatal("scope retry scheduled", child, err)
			}
			var count int
			if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE retry_parent_id=?`, id).Scan(&count); err != nil || count != 0 {
				t.Fatal(count, err)
			}
			if mode != "valid" {
				var status string
				if err = s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, id).Scan(&status); err != nil || status != "running" {
					t.Fatal("invalid policy changed original attempt", status, err)
				}
				return
			}
			var info string
			if err = s.DB.QueryRow(`SELECT retry_info_json FROM platform_runs WHERE id=?`, id).Scan(&info); err != nil {
				t.Fatal(err)
			}
			var retry RetryInfo
			if err = json.Unmarshal([]byte(info), &retry); err != nil || retry.State != "repository_unavailable" {
				t.Fatal(info, err)
			}
			if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE retry_parent_id=?`, id).Scan(&count); err != nil || count != 0 {
				t.Fatal(count, err)
			}
		})
	}
}

func TestRepositoryScopePublicationMakesNoRemoteRequest(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	snap := repositoryScopeSnapshot(repositoryScopeFixture())
	policy := snap.AuditPolicy
	snap.AuditPolicy = nil
	id, _, err := s.EnqueueUser(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueRunCheck(ctx, id, admin.ID, false); err != nil {
		t.Fatal(err)
	}
	injectRepositoryScope(t, s, id, policy)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	settings := &SettingsService{}
	cfg := Settings{}
	cfg.GitLab.URL = server.URL
	cfg.GitLab.Token = "fixture"
	settings.current.Store(&cfg)
	runner := &Runner{Store: s, Settings: settings}
	if claimed, err := runner.DispatchRunCheck(ctx); err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	var state, code string
	if err = s.DB.QueryRow(`SELECT state,code FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state, &code); err != nil || state != "failed" || code != "repository_unavailable" || requests.Load() != 0 {
		t.Fatal(state, code, requests.Load(), err)
	}
}

func TestRepositoryScopeContextIndexConsistency(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	scope := repositoryScopeFixture()
	ref := scope.Target
	ref.ProjectID = 2
	ref.RemoteID = 72
	ref.FullName = "org/context"
	scope.Context = []RepositoryContextReference{{Repository: ref, SHA: scope.Observation.HeadSHA}}
	snap := repositoryScopeSnapshot(scope)
	legacy := snap
	legacy.AuditPolicy = nil
	id, _, err := s.EnqueueUser(ctx, legacy, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateRepositoryScopeContextIndex(ctx, s.DB, id, snap); !errors.Is(err, ErrRepositoryScope) {
		t.Fatal("missing index", err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,?,?)`, id, 2, scope.Context[0].SHA); err != nil {
		t.Fatal(err)
	}
	if err = validateRepositoryScopeContextIndex(ctx, s.DB, id, snap); err != nil {
		t.Fatal("matching index", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_run_context_repositories SET sha='wrong' WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = validateRepositoryScopeContextIndex(ctx, s.DB, id, snap); !errors.Is(err, ErrRepositoryScope) {
		t.Fatal("wrong SHA", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_run_context_repositories SET sha=? WHERE run_id=?`, scope.Context[0].SHA, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,?,?)`, id, 3, scope.Context[0].SHA); err != nil {
		t.Fatal(err)
	}
	if err = validateRepositoryScopeContextIndex(ctx, s.DB, id, snap); !errors.Is(err, ErrRepositoryScope) {
		t.Fatal("extra index", err)
	}
}
