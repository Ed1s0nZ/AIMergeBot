package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	legacy "pr_agent/internal"
)

func contextFixture(t *testing.T) (*Store, User, User, Snapshot) {
	t.Helper()
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	items := []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("a", 40)}}
	if err := s.SaveContextRepositories(ctx, 1, items, admin.ID); err != nil {
		t.Fatal(err)
	}
	return s, admin, member, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("b", 40), HeadSHA: strings.Repeat("c", 40), AuditPolicy: &AuditPolicy{ContextRepositories: items}}
}

func TestContextConfigurationAdminValidationAndDurablePrivateSync(t *testing.T) {
	s, admin, member, snap := contextFixture(t)
	ctx := context.Background()
	for _, items := range [][]ContextRepository{
		{{ProjectID: 1, SHA: strings.Repeat("a", 40)}},
		{{ProjectID: 2, SHA: "main"}},
		{{ProjectID: 2, SHA: strings.Repeat("a", 40)}, {ProjectID: 2, SHA: strings.Repeat("b", 40)}},
		{{ProjectID: 2, SHA: strings.Repeat("a", 40)}, {ProjectID: 3, SHA: strings.Repeat("a", 40)}, {ProjectID: 4, SHA: strings.Repeat("a", 40)}, {ProjectID: 5, SHA: strings.Repeat("a", 40)}, {ProjectID: 6, SHA: strings.Repeat("a", 40)}},
	} {
		if err := s.SaveContextRepositories(ctx, 1, items, admin.ID); !errors.Is(err, ErrContextRepository) {
			t.Fatal("invalid configuration accepted", err)
		}
	}
	if err := s.SaveContextRepositories(ctx, 1, nil, member.ID); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("operator edited repository grants", err)
	}
	if err := s.SaveContextRepositories(ctx, 1, []ContextRepository{{ProjectID: 99, SHA: strings.Repeat("a", 40)}}, admin.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("unknown repository accepted", err)
	}
	items, err := s.ContextRepositories(ctx, 1)
	if err != nil || len(items) != 1 || items[0] != snap.AuditPolicy.ContextRepositories[0] {
		t.Fatal("failed transaction erased grants", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	svc, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.OpenAI.APIKey = "synthetic-context-private-key"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, svc); err != nil {
		t.Fatal(err)
	}
	copy := svc.Snapshot()
	copy.Projects[0].ContextRepositories[0].SHA = "modified-copy"
	if svc.Snapshot().Projects[0].ContextRepositories[0].SHA != items[0].SHA {
		t.Fatal("snapshot exposes mutable link slice")
	}
	reopened, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().OpenAI.APIKey != cfg.OpenAI.APIKey || len(reopened.Snapshot().Projects[0].ContextRepositories) != 1 {
		t.Fatal("sync lost credential or grant")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions", err)
	}
	// Legacy imports cannot silently erase newly managed grants.
	if err = s.ImportContextRepositories(ctx, []legacy.ProjectConfig{{ID: 1}}); err != nil {
		t.Fatal(err)
	}
	items, err = s.ContextRepositories(ctx, 1)
	if err != nil || len(items) != 1 {
		t.Fatal("old config erased links", err)
	}
	if err = s.SaveContextRepositories(ctx, 1, nil, admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if len(svc.Snapshot().Projects[0].ContextRepositories) != 0 {
		t.Fatal("clear not synchronized")
	}
}

func TestContextRunPermissionIntersectionAndRevocation(t *testing.T) {
	s, admin, member, snap := contextFixture(t)
	ctx := context.Background()
	if _, _, err := s.EnqueueUser(ctx, snap, member.ID, false); err == nil {
		t.Fatal("target-only user submitted cross-repository audit")
	}
	if err := s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	id, _, err := s.EnqueueUser(ctx, snap, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	var dependencies int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_run_context_repositories WHERE run_id=?`, id).Scan(&dependencies); err != nil || dependencies != 1 {
		t.Fatal("dependency not atomic", err)
	}
	if _, err = requireSnapshotRole(ctx, s.DB, snap, member.ID, "operator"); err != nil {
		t.Fatal("context viewer incorrectly needs operator", err)
	}
	s.Claim(ctx)
	if err = s.Checkpoint(ctx, id, AuditResult{Findings: []Finding{{ID: "retained"}}, Summary: "checkpoint"}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "cancelled" || len(run.Result.Findings) != 1 {
		t.Fatal("revocation lost or failed cancellation", err)
	}
	if _, err = requireSnapshotRole(ctx, s.DB, run.Snapshot, member.ID, "viewer"); err == nil {
		t.Fatal("revoked user still reads retained cross-source report")
	}
	if _, _, err = s.EnqueueUser(ctx, snap, member.ID, true); err == nil {
		t.Fatal("revoked viewer re-enqueued")
	}
	if err = s.SaveContextRepositories(ctx, 1, nil, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Enqueue(ctx, snap, 0, true); !errors.Is(err, ErrContextRepository) {
		t.Fatal("trusted ingress bypassed revoked grant", err)
	}
}

func TestContextRetryDependenciesCommentIsolationAndGrantReplacement(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	child, err := s.FailAndRetry(ctx, id, "transient", AuditResult{}, nil, 0)
	if err != nil || child == 0 {
		t.Fatal("retry", err)
	}
	var sha string
	if err = s.DB.QueryRow(`SELECT sha FROM platform_run_context_repositories WHERE run_id=?`, child).Scan(&sha); err != nil || sha != snap.AuditPolicy.ContextRepositories[0].SHA {
		t.Fatal("retry lost frozen dependency", err)
	}
	s.Claim(ctx)
	if err = s.Finish(ctx, child, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f"}}, Summary: "cross-source fixture"}, nil); err != nil {
		t.Fatal(err)
	}
	delivery, err := s.CommentDelivery(ctx, child)
	if err != nil || delivery.State != "blocked" || !strings.Contains(delivery.LastError, "permission intersection") {
		t.Fatal("cross-source report queued public comment", err)
	}
	if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: child, FindingID: "f", Status: "accepted", Actor: admin.ID})); err != nil {
		t.Fatal(err)
	}
	delivery, err = s.CommentDelivery(ctx, child)
	if err != nil || delivery.State != "blocked" {
		t.Fatal("review unblocked cross-source publication", err)
	}
	snap.MRIID = 2
	active, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	changed := []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("b", 40)}}
	if err = s.SaveContextRepositories(ctx, 1, changed, admin.ID); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, active)
	if err != nil || run.Status != "cancelled" {
		t.Fatal("changed grant did not cancel stale reader", err)
	}
}

func TestContextHTTPAdminGateAndReportListIntersection(t *testing.T) {
	s, admin, member, snap := contextFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := get("/api/v1/projects/1/context-repositories"); res.Code != 403 {
		t.Fatal("member enumerated admin grants", res.Code)
	}
	if res := get(fmt.Sprintf("/api/v1/runs/%d", id)); res.Code == 200 {
		t.Fatal("target-only user read cross-source details")
	}
	list := get("/api/v1/runs")
	var body struct {
		Total int `json:"total"`
	}
	if err = json.Unmarshal(list.Body.Bytes(), &body); err != nil || body.Total != 0 {
		t.Fatal("cross-source task leaked through list", err)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if res := get(fmt.Sprintf("/api/v1/runs/%d", id)); res.Code != 200 {
		t.Fatal("authorized intersection cannot read", res.Code)
	}
	list = get("/api/v1/runs")
	if err = json.Unmarshal(list.Body.Bytes(), &body); err != nil || body.Total != 1 {
		t.Fatal("authorized task absent", err)
	}
}

func TestContextHistoryNeverBridgesDifferentRepositorySets(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	f := Finding{ID: "first", File: "file.any", Side: "head", Line: 1, Type: "risk", Evidence: "danger(input)", Trigger: "input"}
	first := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: first.ID, FindingID: f.ID, Status: "accepted", Reason: "private related-source conclusion", Actor: admin.ID})); err != nil {
		t.Fatal(err)
	}
	snap.HeadSHA = strings.Repeat("d", 40)
	snap.AuditPolicy = &AuditPolicy{}
	f.ID = "second"
	second := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	history, err := s.FindingLifecycle(ctx, second)
	if err != nil || len(history.Current) != 1 || len(history.Current[0].Occurrences) != 1 || len(history.Current[0].Reviews) != 0 {
		t.Fatal("old private receipt bridged different context", err)
	}
}

func TestContextHTTPReplacementInterruptsAndSyncFailureRemainsRecoverable(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	interrupted := make(chan struct{}, 1)
	runner := &Runner{Store: s, active: map[int64]context.CancelFunc{id: func() { interrupted <- struct{}{} }}}
	path := filepath.Join(t.TempDir(), "config.yaml")
	settings, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "admin-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s, Runner: runner, Settings: settings}).Register(router)
	body := fmt.Sprintf(`{"items":[{"project_id":2,"sha":"%s"}]}`, strings.Repeat("b", 40))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/1/context-repositories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var result struct {
		Pending bool `json:"config_sync_pending"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Pending {
		t.Fatal("durable write confused with config replace", response.Code)
	}
	select {
	case <-interrupted:
	default:
		t.Fatal("running context reader not interrupted")
	}
	items, err := s.ContextRepositories(ctx, 1)
	if err != nil || len(items) != 1 || items[0].SHA != strings.Repeat("b", 40) {
		t.Fatal("failed config sync lost committed grant", err)
	}
	allowImport, err := s.RestorePendingProjects(ctx, settings)
	if err == nil || allowImport {
		t.Fatal("stale config allowed to overwrite grant")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if settings.Snapshot().Projects[0].ContextRepositories[0].SHA != items[0].SHA {
		t.Fatal("recovery lost grant version")
	}
}

func TestDisablingContextProjectCancelsDependentRunsAndPreservesConfiguration(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 2, Name: "context", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "cancelled" {
		t.Fatal("disabled source still scheduled", err)
	}
	if _, _, err = s.Enqueue(ctx, snap, 0, true); !errors.Is(err, ErrContextRepository) {
		t.Fatal("disabled context admitted", err)
	}
	items, err := s.ContextRepositories(ctx, 1)
	if err != nil || len(items) != 1 {
		t.Fatal("disabled source erased authorized config", err)
	}
	// Restart may restore a configured but disabled source, without admitting reads.
	if err = s.ImportContextRepositories(ctx, []legacy.ProjectConfig{{ID: 1, ContextRepositories: items}}); err != nil {
		t.Fatal("disabled config prevents restart", err)
	}
}

func TestContextDependencyMigrationBackfillsOnceWithoutRestoringRevokedGrants(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-projection database carrying immutable context policy JSON.
	if _, err = s.DB.Exec(`DELETE FROM platform_run_context_repositories WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_access_migrations WHERE name='context-repositories-v1'`); err != nil {
		t.Fatal(err)
	}
	var sequence int
	var databaseName, path string
	if err = s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &databaseName, &path); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	var sha string
	if err = reopened.DB.QueryRow(`SELECT sha FROM platform_run_context_repositories WHERE run_id=?`, id).Scan(&sha); err != nil || sha != snap.AuditPolicy.ContextRepositories[0].SHA {
		t.Fatal("dependency migration lost fixed scope", err)
	}
	if err = reopened.SaveContextRepositories(ctx, 1, nil, admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = reopened.migrate(); err != nil {
		t.Fatal(err)
	}
	items, err := reopened.ContextRepositories(ctx, 1)
	if err != nil || len(items) != 0 {
		t.Fatal("migration restored revoked grant", err)
	}
	var markerCount int
	if err = reopened.DB.QueryRow(`SELECT COUNT(*) FROM platform_access_migrations WHERE name='context-repositories-v1'`).Scan(&markerCount); err != nil || markerCount != 1 {
		t.Fatal("migration marker not idempotent", err)
	}
}
