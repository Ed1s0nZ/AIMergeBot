package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckRunnerStartStopAndRestart(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// The fixture uses in-memory settings, so no configuration file needs
	// synchronization. Exercise the real worker and check loops below.
	if _, err := s.DB.Exec(`UPDATE platform_project_sync SET dirty=0`); err != nil {
		t.Fatal(err)
	}
	policy, err := s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{Checks: &CheckRules{Enabled: true, Mode: "advisory"}})
	if err != nil {
		t.Fatal(err)
	}
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"source_project_id": 1, "source_branch": "feature", "diff_refs": map[string]string{"head_sha": head, "base_sha": base}})
			return
		}
		posts.Add(1)
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 43, "sha": head, "name": payload["name"], "status": payload["state"]})
	}))
	defer server.Close()
	cfg := Settings{}
	cfg.GitLab.URL, cfg.GitLab.Token = server.URL, "fixture"
	settings := &SettingsService{}
	settings.current.Store(&cfg)
	runner := &Runner{Store: s, Settings: settings, Repository: runRepo{}, Workers: 1}
	defer runner.Stop()
	create := func(mr int) int64 {
		t.Helper()
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, HeadSHA: head, BaseSHA: base, AuditPolicy: &AuditPolicy{RepositoryURL: server.URL, Workflow: &policy}}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	waitPublished := func(id int64, want int32) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			var state string
			if s.DB.QueryRow(`SELECT state FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state) == nil && state == "published" {
				if posts.Load() != want {
					t.Fatal("duplicate remote publication", posts.Load(), want)
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("background check did not publish", id, posts.Load())
	}
	first := create(1)
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitPublished(first, 1)
	stopped := make(chan struct{})
	go func() { runner.Stop(); runner.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("check loop prevented shutdown")
	}
	second := create(2)
	// Observe beyond one complete check interval before restarting. An
	// orphaned timer must not collect or send this newly authorized result.
	time.Sleep(2200 * time.Millisecond)
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_check_deliveries WHERE run_id=?`, second).Scan(&count); err != nil || count != 0 || posts.Load() != 1 {
		t.Fatal("stopped runner collected", count, err)
	}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitPublished(second, 2)
}
