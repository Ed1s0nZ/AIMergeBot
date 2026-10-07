package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRepositoryBindingHTTPContract(t *testing.T) {
	s, administrator, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "viewer", administrator.ID); err != nil {
		t.Fatal(err)
	}
	integration := bindingIntegration(t, s, administrator.ID, "github", "https://api.example", 1, 2)
	repo := &bindingGuardRepository{}
	router := gin.New()
	(&HTTP{Store: s, Runner: &Runner{Store: s, Repository: repo}}).Register(router)
	request := func(method, path, token, body, origin, site string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "fixture-private-token") {
			t.Fatal("credential leaked")
		}
		if res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("response cacheable", res.Header())
		}
		return res
	}
	login := func(username string) string {
		t.Helper()
		res := request("POST", "/api/v1/auth/login", "", fmt.Sprintf(`{"username":%q,"password":%q}`, username, username+"-long-password"), "", "", 200)
		for _, c := range res.Result().Cookies() {
			if c.Name == sessionCookie {
				return c.Value
			}
		}
		t.Fatal("login omitted session")
		return ""
	}
	adminToken, memberToken := login("admin"), login("member")
	path := "/api/v1/projects/1/repository-binding"
	body := fmt.Sprintf(`{"expected_revision":0,"provider":"github","api_origin":"https://API.EXAMPLE:443/","remote_id":7,"full_name":"org/repo","integration_id":%d}`, integration.ID)
	request("GET", path, "", "", "", "", 401)
	request("PATCH", path, "", body, "", "", 401)
	request("PATCH", path, memberToken, body, "", "", 403)
	request("PATCH", path, adminToken, body, "https://attacker.example", "", 403)
	request("PATCH", path, adminToken, body, "", "cross-site", 403)
	request("GET", "/api/v1/projects/2/repository-binding", memberToken, "", "", "", 404)
	request("GET", "/api/v1/projects/999/repository-binding", adminToken, "", "", "", 404)
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		request("GET", "/api/v1/projects/"+id+"/repository-binding", adminToken, "", "", "", 400)
	}
	for _, invalid := range []string{
		`{}`, `null`, `[]`, strings.Replace(body, `"expected_revision":0,`, "", 1),
		strings.Replace(body, `"expected_revision":0`, `"expected_revision":null`, 1),
		strings.Replace(body, `"expected_revision":0`, `"expected_revision":-1`, 1),
		strings.Replace(body, `"expected_revision":0`, `"expected_revision":9223372036854775807`, 1),
		strings.Replace(body, `"remote_id":7`, `"remote_id":0`, 1),
		strings.Replace(body, `"remote_id":7`, `"remote_id":"7"`, 1),
		strings.Replace(body, `"org/repo"`, `"../repo"`, 1),
		strings.Replace(body, `"github"`, `"other"`, 1),
		strings.Replace(body, `https://API.EXAMPLE:443/`, `https://user:secret@api.example`, 1),
		strings.TrimSuffix(body, "}") + `,"revision":100}`,
		strings.TrimSuffix(body, "}") + `,"actor":999}`,
		strings.TrimSuffix(body, "}") + `,"token":"private-input"}`,
		strings.TrimSuffix(body, "}") + `,"padding":"` + strings.Repeat("x", 8192) + `"}`,
		body + ` {}`, body + ` null`, body + ` garbage`,
	} {
		res := request("PATCH", path, adminToken, invalid, "", "", 400)
		if strings.Contains(res.Body.String(), "secret") || strings.Contains(res.Body.String(), "private-input") {
			t.Fatal("input echoed", res.Body.String())
		}
	}
	var history int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&history); err != nil || history != 0 {
		t.Fatal("rejected request wrote history", history, err)
	}
	initial := request("GET", path, memberToken, "", "", "", 200)
	var binding RepositoryBinding
	if err := json.Unmarshal(initial.Body.Bytes(), &binding); err != nil || binding.Revision != 0 {
		t.Fatal(binding, err)
	}
	saved := request("PATCH", path, adminToken, body, "http://example.com", "same-origin", 200)
	if err := json.Unmarshal(saved.Body.Bytes(), &binding); err != nil || binding.Revision != 1 || binding.APIOrigin != "https://api.example" {
		t.Fatal(binding, err)
	}
	read := request("GET", path, memberToken, "", "", "", 200)
	if read.Body.String() != saved.Body.String() {
		t.Fatal("saved binding differs from read", read.Body.String())
	}
	request("PATCH", path, adminToken, body, "", "", 409)
	request("PATCH", "/api/v1/projects/2/repository-binding", adminToken, body, "", "", 409)
	next := strings.Replace(body, `"expected_revision":0`, `"expected_revision":1`, 1)
	request("PATCH", path, adminToken, strings.Replace(next, fmt.Sprintf(`"integration_id":%d`, integration.ID), `"integration_id":999`, 1), "", "", 404)
	request("PATCH", path, adminToken, strings.Replace(next, "https://API.EXAMPLE:443/", "https://wrong.example", 1), "", "", 409)
	request("PATCH", "/api/v1/projects/999/repository-binding", adminToken, body, "", "", 404)
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_http_binding_event BEFORE INSERT ON platform_events WHEN NEW.action='repository.binding.updated' BEGIN SELECT RAISE(ABORT,'private-storage-detail'); END`); err != nil {
		t.Fatal(err)
	}
	rejected := request("PATCH", path, adminToken, next, "", "", 500)
	if strings.Contains(rejected.Body.String(), "private-storage-detail") {
		t.Fatal("storage error leaked", rejected.Body.String())
	}
	if request("GET", path, memberToken, "", "", "", 200).Body.String() != saved.Body.String() {
		t.Fatal("failed event write changed binding")
	}
	if _, err := s.DB.Exec(`DROP TRIGGER reject_http_binding_event`); err != nil {
		t.Fatal(err)
	}
	res := request("POST", "/api/v1/runs", adminToken, `{"project_id":1,"mr_iid":1}`, "", "", 503)
	if !strings.Contains(res.Body.String(), `"code":"repository_unavailable"`) || res.Header().Get("Retry-After") != "" || repo.snapshots.Load() != 0 {
		t.Fatal("bound submission used legacy reader", res.Body.String(), repo.snapshots.Load())
	}
	if err := s.SetProjectMember(ctx, 1, member.ID, "", administrator.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", path, memberToken, "", "", "", 404)
	var actor int64
	if err := s.DB.QueryRow(`SELECT actor FROM platform_repository_binding_history WHERE project_id=1 AND revision=1`).Scan(&actor); err != nil || actor != administrator.ID {
		t.Fatal(actor, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&history); err != nil || history != 1 {
		t.Fatal(history, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_project_members WHERE user_id=?`, member.ID).Scan(&history); err != nil || history != 0 {
		t.Fatal("binding granted access", history, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&history); err != nil || history != 0 {
		t.Fatal("bound request created run", history, err)
	}
}
