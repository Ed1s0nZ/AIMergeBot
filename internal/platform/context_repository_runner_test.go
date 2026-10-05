package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestContextPreparationUsesFixedGitLabCommitAndRevocationStopsCachedReads(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	var reads atomic.Int32
	sha := snap.AuditPolicy.ContextRepositories[0].SHA
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/2/repository/commits/" + sha:
			json.NewEncoder(w).Encode(map[string]string{"id": sha})
		case "/api/v4/projects/2/repository/files/guard.any":
			if r.URL.Query().Get("ref") != sha {
				t.Error("moving or incorrect context ref")
			}
			reads.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"file_path": "guard.any", "encoding": "base64", "size": 16, "content": base64.StdEncoding.EncodeToString([]byte("context pinned\n"))})
		default:
			t.Error("unexpected context destination", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	settings, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.Snapshot()
	cfg.GitLab.URL = server.URL
	cfg.GitLab.Token = "synthetic"
	cfg.GitAudit.Enabled = false
	if err = settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	snap.AuditPolicy.RepositoryURL = server.URL
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	run, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Settings: settings}
	sources, notes, cleanup, err := runner.prepareContextSources(ctx, run, cfg.GitAudit, nil)
	if err != nil || len(notes) != 0 {
		t.Fatal("prepare context", err, notes)
	}
	defer cleanup()
	tools := crossTools(runRepo{}, run.Snapshot, sources)
	out, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	if out.Error != "" || !strings.Contains(out.Text, "context pinned") || out.HeadSHA != sha {
		t.Fatal("API fixed source", out.Error)
	}
	if err = s.SaveContextRepositories(ctx, 1, nil, admin.ID); err != nil {
		t.Fatal(err)
	}
	denied, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	if denied.Error == "" || denied.Text != "" || reads.Load() != 1 {
		t.Fatal("cached read bypassed live grant check")
	}
}

func TestContextUnavailableCommitIsDisclosedAndNotReplacedByLatest(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": strings.Repeat("b", 40)})
	}))
	defer server.Close()
	settings, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.Snapshot()
	cfg.GitLab.URL = server.URL
	cfg.GitAudit.Enabled = false
	if err = settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	snap.AuditPolicy.RepositoryURL = server.URL
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	run, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Settings: settings}
	sources, notes, cleanup, err := runner.prepareContextSources(ctx, run, cfg.GitAudit, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(notes) != 1 || sources[2].Repository != nil {
		t.Fatal("wrong commit silently accepted", notes)
	}
	listed, _ := crossTools(runRepo{}, run.Snapshot, sources).contextRepositories(ctx, struct{}{})
	if listed.Error != "" || len(listed.Repositories) != 1 || listed.Repositories[0].Available {
		t.Fatal("unavailable fixed object not exposed")
	}
}

func TestSubmitFreezesAdministratorContextSetAndAdmissionRechecksIt(t *testing.T) {
	s, admin, member, _ := contextFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Repository: runRepo{}}
	id, _, err := runner.Submit(ctx, 1, 1, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := s.Run(ctx, id)
	if err != nil || len(contextPolicyItems(frozen.Snapshot)) != 1 {
		t.Fatal("submit failed to freeze context", err)
	}
	old := frozen.AuditPolicy.ContextRepositories[0].SHA
	if err = s.SaveContextRepositories(ctx, 1, []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("b", 40)}}, admin.ID); err != nil {
		t.Fatal(err)
	}
	again, _, err := runner.Submit(ctx, 1, 2, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	newRun, err := s.Run(ctx, again)
	if err != nil || newRun.AuditPolicy.ContextRepositories[0].SHA == old {
		t.Fatal("new submission inherited stale context", err)
	}
	original, err := s.Run(ctx, id)
	if err != nil || original.AuditPolicy.ContextRepositories[0].SHA != old {
		t.Fatal("configuration update mutated immutable snapshot", err)
	}
}

func TestPollingFreezesContextAndDoesNotMarkRevokedAdmissionSeen(t *testing.T) {
	s, _, runner, token := quotaIngressFixture(t)
	ctx := context.Background()
	admin, err := s.Session(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 2, Name: "context", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	if err = s.SaveContextRepositories(ctx, 1, []ContextRepository{{ProjectID: 2, SHA: sha}}, admin.ID); err != nil {
		t.Fatal(err)
	}
	// Avoid polling the context project's own MRs in this fixture. Its enabled
	// state is still enforced by the transaction that admits the primary task.
	if _, err = s.DB.ExecContext(ctx, `UPDATE platform_projects SET enabled=0 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	initialized := map[int]bool{}
	runner.pollCycle(ctx, initialized)
	var seen int
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_poll_seen WHERE project_id=1`).Scan(&seen); err != nil || seen != 0 || initialized[1] {
		t.Fatal("rejected context marked seen", err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE platform_projects SET enabled=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	original := quotaGitLabFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/2/merge_requests" {
			json.NewEncoder(w).Encode([]any{})
			return
		}
		original.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	cfg := runner.Settings.Snapshot()
	cfg.GitLab.URL = server.URL
	if err = runner.Settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	runner.pollCycle(ctx, initialized)
	var id int64
	if err = s.DB.QueryRowContext(ctx, `SELECT id FROM platform_runs WHERE project_id=1 AND status='pending'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, id)
	if err != nil || len(contextPolicyItems(run.Snapshot)) != 1 || contextPolicyItems(run.Snapshot)[0].SHA != sha {
		t.Fatal("poll policy did not freeze context", err)
	}
	if !initialized[1] {
		t.Fatal("successful poll remained uninitialized")
	}
}
