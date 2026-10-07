package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHTTPAuthorizationAndSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "member", "member-password", "member"); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.SetTrustedProxies(nil)
	h := &HTTP{Store: s, Runner: &Runner{Store: s}}
	h.Register(r)
	request := func(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := request("GET", "/api/v1/integrations", "", nil, ""); w.Code != 401 {
		t.Fatal("unauthenticated integrations access")
	}
	if w := request("GET", "/api/v1/projects", "", nil, ""); w.Code != 401 {
		t.Fatal("unauthenticated access")
	}
	login := request("POST", "/api/v1/auth/login", `{"username":"member","password":"member-password"}`, nil, "")
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe cookie")
	}
	cookie := cookies[0]
	if w := request("GET", "/api/v1/integrations", "", cookie, ""); w.Code != 403 {
		t.Fatal("member integrations access allowed")
	}
	if w := request("POST", "/api/v1/integrations", `{}`, cookie, ""); w.Code != 403 {
		t.Fatal("member integrations mutation allowed")
	}
	if w := request("POST", "/api/v1/projects", `{"id":1,"name":"test","enabled":true}`, cookie, ""); w.Code != 403 {
		t.Fatal("member config allowed")
	}
	if w := request("POST", "/api/v1/auth/logout", `{}`, cookie, "https://evil.example"); w.Code != 403 {
		t.Fatal("cross-origin write allowed")
	}
	if w := request("GET", "/api/v1/auth/me", "", cookie, ""); w.Code != 200 {
		t.Fatal("valid session rejected")
	}
	if w := request("POST", "/api/v1/auth/logout", `{}`, cookie, ""); w.Code != 204 {
		t.Fatal("logout failed")
	}
	if w := request("GET", "/api/v1/auth/me", "", cookie, ""); w.Code != 401 {
		t.Fatal("revoked session allowed")
	}
	if w := request("POST", "/webhook", `{}`, nil, ""); w.Code != 401 {
		t.Fatal("unsigned webhook allowed")
	}
}

func TestEinoUsesToolsAndValidatesEvidence(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected model path %s", r.URL.Path)
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			}
			Tools []any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		assertRequiredToolCapabilities(t, req.Tools, "read_file", "search_code", "get_diff", "record_hypothesis", "update_investigation", "submit_finding", "resolve_recording_errors")
		w.Header().Set("Content-Type", "application/json")
		n := calls.Add(1)
		if n == 3 && (len(req.Messages) == 0 || !strings.Contains(fmt.Sprint(req.Messages[0].Content), primaryRecordingFinalizationRequest)) {
			t.Error("extra call is not the bounded recording final")
		}
		if n == 1 {
			fmt.Fprint(w, `{"id":"t1","object":"chat.completion","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.go\",\"start\":1,\"end\":3}"}}]}}]}`)
			return
		}
		found := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				found = true
			}
		}
		if !found {
			t.Error("no tool observation in model conversation")
		}
		content := `{"findings":[{"id":"","file":"a.go","line":2,"severity":"high","type":"unsafe call","title":"unsafe call","description":"untrusted input reaches sink","evidence":"danger(input)","trigger":"external input without guard","suggestion":"validate input","confidence":"candidate"}],"summary":"One candidate needs review","coverage_notes":[]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t2", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	defer server.Close()
	repo := fixtureRepo{files: map[string]string{"a.go": "safe\ndanger(input)\n"}}
	scope := BuildDiff([]Change{{NewPath: "a.go", Diff: "@@ -1 +1,2 @@\n safe\n+danger(input)"}}, nil, 10000)
	auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "fixture", BaseURL: server.URL, Model: "fixture-tool-model", MaxSteps: 10}}
	result, trace, err := auditor.Audit(context.Background(), Snapshot{HeadSHA: "pinned"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace) < 1 || len(result.Findings) != 1 {
		t.Fatalf("Eino path incomplete: %+v %+v", result, trace)
	}
	if calls.Load() != 3 { // Source, early final, and one bounded recording final.
		t.Fatal("unexpected model loop")
	}
}

func TestAuditHTTPWorkflowAndFilters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		content := `{"findings":[{"id":"","file":"a.go","line":1,"severity":"high","type":"unsafe change","title":"unsafe change","description":"candidate","evidence":"new","trigger":"external input","suggestion":"validate","confidence":"candidate"}],"summary":"one candidate","coverage_notes":[]}`
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	defer modelServer.Close()
	repo := runRepo{}
	runner := &Runner{Store: s, Repository: repo, Auditor: &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "fixture", BaseURL: modelServer.URL, Model: "fixture", MaxSteps: 6}}, Workers: 1, Timeout: 3 * time.Second}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Runner: runner}).Register(router)
	var cookie *http.Cookie
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	login := request("POST", "/api/v1/auth/login", `{"username":"admin","password":"a-long-password"}`)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie = login.Result().Cookies()[0]
	if w := request("POST", "/api/v1/projects", `{"id":1,"name":"fixture","enabled":true}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	submitted := request("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":1}`)
	if submitted.Code != 202 {
		t.Fatal(submitted.Body.String())
	}
	var response struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, response.ID, "incomplete")
	detail := request("GET", fmt.Sprintf("/api/v1/runs/%d", response.ID), "")
	var payload struct {
		Run Run `json:"run"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &payload); err != nil || len(payload.Run.Result.Findings) != 1 {
		t.Fatal("result missing", err)
	}
	if len(payload.Run.Result.CoverageNotes) != 2 || !hasPlanGap(payload.Run.Result.CoverageNotes) || !strings.Contains(payload.Run.Result.CoverageNotes[1], "PR impact recording gap") {
		t.Fatal("persisted incomplete reason missing", payload.Run.Result)
	}
	f := payload.Run.Result.Findings[0]
	review := request("PUT", fmt.Sprintf("/api/v1/runs/%d/findings/%s/review", response.ID, f.ID), `{"status":"false_positive","reason":"guard exists","expected_revision":0}`)
	if review.Code != 204 {
		t.Fatal(review.Body.String())
	}
	for _, test := range []struct {
		q     string
		total int
	}{{"project_id=1&level=high&type=unsafe+change&review_status=false_positive", 1}, {"project_id=2", 0}, {"level=low", 0}, {"review_status=pending", 0}} {
		w := request("GET", "/api/v1/runs?"+test.q, "")
		var list struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != 200 || list.Total != test.total {
			t.Fatalf("filter %s: %s", test.q, w.Body.String())
		}
	}
	// A retry keeps the first run's review intact.
	retry := request("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":1,"force":true}`)
	if retry.Code != 202 {
		t.Fatal(retry.Body.String())
	}
	reviews, err := s.Reviews(ctx, response.ID)
	if err != nil || len(reviews) != 1 {
		t.Fatal("retry erased review")
	}
}

func TestSettingsHTTPWithTLSProxyOrigin(t *testing.T) {
	dir := t.TempDir()
	svc, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.PublicURL = "https://audit.example.com"
	cfg.OpenAI.APIKey = "fixture-secret"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	ctx := context.Background()
	if err = s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s, Runner: &Runner{Store: s}, Settings: svc}).Register(router)
	req := httptest.NewRequest("POST", "http://backend:8080/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://audit.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure {
		t.Fatal("TLS proxy session lacks Secure")
	}
	req = httptest.NewRequest("GET", "http://backend:8080/api/v1/settings", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "fixture-secret") {
		t.Fatal("settings exposed secret")
	}
	var data map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	data["openai"].(map[string]any)["model"] = "changed-through-http"
	raw, _ := json.Marshal(data)
	req = httptest.NewRequest("PUT", "http://backend:8080/api/v1/settings", strings.NewReader(string(raw)))
	req.AddCookie(cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://audit.example.com")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	reloaded, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil || reloaded.Snapshot().OpenAI.Model != "changed-through-http" || reloaded.Snapshot().OpenAI.APIKey != "fixture-secret" {
		t.Fatal("settings file not synchronized", err)
	}
}
