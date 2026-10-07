package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAutomaticCheckCycleOptInRevocationAndNoReplay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"source_project_id": 1, "source_branch": "feature", "diff_refs": map[string]string{"head_sha": head, "base_sha": base}})
			return
		}
		posts++
		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 43, "sha": head, "name": payload["name"], "status": payload["state"]})
	}))
	defer server.Close()
	cfg := Settings{}
	cfg.GitLab.URL = server.URL
	cfg.GitLab.Token = "fixture"
	settings := &SettingsService{}
	settings.current.Store(&cfg)
	runner := &Runner{Store: s, Settings: settings}
	create := func(mr int, policy *WorkflowPolicy) int64 {
		t.Helper()
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, HeadSHA: head, BaseSHA: base, AuditPolicy: &AuditPolicy{RepositoryURL: server.URL, Workflow: policy}}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	create(1, nil)
	runner.checkCycle(ctx)
	if posts != 0 {
		t.Fatal("default checks published", posts)
	}
	policy, err := s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{Checks: &CheckRules{Enabled: true, Mode: "advisory"}})
	if err != nil {
		t.Fatal(err)
	}
	id := create(2, &policy)
	runner.checkCycle(ctx)
	var state string
	if err := s.DB.QueryRow(`SELECT state FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state); err != nil || state != "published" || posts != 1 {
		t.Fatal("authorized cycle not published", state, posts, err)
	}
	runner.checkCycle(ctx)
	if posts != 1 {
		t.Fatal("cycle replayed terminal publication", posts)
	}
	old := create(3, &policy)
	policy.Checks.Enabled = false
	if policy, err = s.SaveWorkflowPolicy(ctx, 1, 1, policy.Revision, policy); err != nil {
		t.Fatal(err)
	}
	runner.checkCycle(ctx)
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_check_deliveries WHERE run_id=?`, old).Scan(&count); err != nil || count != 0 || posts != 1 {
		t.Fatal("revoked authorization published", count, posts, err)
	}
	policy.Checks.Enabled = true
	if policy, err = s.SaveWorkflowPolicy(ctx, 1, 1, policy.Revision, policy); err != nil {
		t.Fatal(err)
	}
	for mr := 4; mr < 10; mr++ {
		create(mr, &policy)
	}
	runner.checkCycle(ctx)
	if posts != 6 {
		t.Fatal("cycle did not enforce five-delivery bound", posts)
	}
	runner.checkCycle(ctx)
	if posts != 7 {
		t.Fatal("remaining delivery did not progress", posts)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	runner.checkLoop(canceled)
}
