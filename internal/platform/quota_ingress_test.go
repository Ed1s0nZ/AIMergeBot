package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func quotaGitLabFixture(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/projects/1/merge_requests":
			fmt.Fprint(w, `[{"iid":1}]`)
		case "/api/v4/projects/1/merge_requests/1":
			fmt.Fprint(w, `{"iid":1,"source_project_id":1,"title":"synthetic quota MR","diff_refs":{"base_sha":"base","head_sha":"head"}}`)
		case "/api/v4/projects/1/merge_requests/1/versions":
			fmt.Fprint(w, `[{"id":1,"base_commit_sha":"base","head_commit_sha":"head"}]`)
		default:
			t.Errorf("unexpected GitLab request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
func quotaIngressFixture(t *testing.T) (*Store, *SettingsService, *Runner, string) {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "admin-quota-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "quota ingress", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	svc, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.GitLab.URL = quotaGitLabFixture(t).URL
	cfg.AuditQuotas.OutstandingGlobal = 1
	cfg.EnableWebhook = true
	cfg.WebhookToken = "synthetic-webhook-token"
	cfg.EnablePolling = true
	cfg.ScanExistingMRs = true
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s.BindQuotaSettings(svc)
	runner := &Runner{Store: s, Settings: svc}
	_, token, err := s.Login(ctx, "admin", "admin-quota-password")
	if err != nil {
		t.Fatal(err)
	}
	return s, svc, runner, token
}
func TestQuotaHTTPAndWebhookRejectExplicitlyAndReuseDuplicates(t *testing.T) {
	s, svc, runner, token := quotaIngressFixture(t)
	ctx := context.Background()
	blocked := enqueueQuota(t, s, 2, 9, 0)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Settings: svc, Runner: runner}).Register(router)
	request := func(webhook bool) *httptest.ResponseRecorder {
		path, body := "/api/v1/runs", `{"project_id":1,"mr_iid":1}`
		if webhook {
			path = "/webhook"
			body = `{"object_kind":"merge_request","project":{"id":1},"object_attributes":{"iid":1,"action":"update"}}`
		}
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if webhook {
			req.Header.Set("X-Gitlab-Token", "synthetic-webhook-token")
			req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
		} else {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, webhook := range []bool{false, true} {
		w := request(webhook)
		var body struct {
			Code, Scope string
			Limit       int
		}
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != 429 || w.Header().Get("Retry-After") != "60" || body.Code != "audit_quota_exceeded" || body.Scope != "outstanding_global" || body.Limit != 1 {
			t.Fatal(webhook, w.Code, w.Body.String())
		}
	}
	if err := s.Cancel(ctx, blocked, 0); err != nil {
		t.Fatal(err)
	}
	if w := request(false); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, webhook := range []bool{false, true} {
		w := request(webhook)
		var body struct{ Created bool }
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != 202 || body.Created {
			t.Fatal("full queue rejected duplicate", webhook, w.Code, w.Body.String())
		}
	}
}
func TestQuotaPollingDoesNotMarkRejectedHeadSeen(t *testing.T) {
	s, _, runner, _ := quotaIngressFixture(t)
	ctx := context.Background()
	blocker := enqueueQuota(t, s, 2, 9, 0)
	initialized := map[int]bool{}
	runner.pollCycle(ctx, initialized)
	var seen int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_poll_seen`).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != 0 || initialized[1] {
		t.Fatal("rejected head marked seen")
	}
	if err := s.Cancel(ctx, blocker, 0); err != nil {
		t.Fatal(err)
	}
	runner.pollCycle(ctx, initialized)
	var head string
	if err := s.DB.QueryRow(`SELECT head_sha FROM platform_poll_seen WHERE project_id=1 AND mr_iid=1`).Scan(&head); err != nil || head != "head" || !initialized[1] {
		t.Fatal("later cycle did not recover", head, err)
	}
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE project_id=1 AND status='pending'`).Scan(&count)
	if count != 1 {
		t.Fatal("poll lost or duplicated audit", count)
	}
}
func TestQuotaSettingsHTTPRejectsExplicitZero(t *testing.T) {
	s, svc, runner, token := quotaIngressFixture(t)
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Settings: svc, Runner: runner}).Register(router)
	payload := svc.Public()
	q := payload["audit_quotas"].(map[string]interface{})
	q["daily_global"] = 0
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", "/api/v1/settings", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_audit_quotas") {
		t.Fatal(w.Code, w.Body.String())
	}
	if svc.Snapshot().AuditQuotas.DailyGlobal != 1000 {
		t.Fatal("bad configuration persisted")
	}
}
