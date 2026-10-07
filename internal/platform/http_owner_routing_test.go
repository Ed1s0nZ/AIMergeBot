package platform

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerRoutingHTTPAccessOriginAndRevision(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	_, adminToken, err := s.Login(ctx, "admin", "admin-long-password")
	if err != nil {
		t.Fatal(err)
	}
	_, memberToken, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	request := func(method, path, token, body, origin string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	path := "/api/v1/projects/1/owner-routing"
	body := fmt.Sprintf(`{"expected_revision":0,"default_owner":%d,"aliases":{"@org/team":[%d]},"actor":%d}`, member.ID, member.ID, member.ID)
	request("GET", path, "", "", "", 401)
	request("PUT", path, memberToken, body, "", 403)
	request("PUT", path, adminToken, body, "https://attacker.example", 403)
	request("GET", "/api/v1/projects/2/owner-routing", memberToken, "", "", 404)
	for _, invalid := range []string{`{}`, `{"expected_revision":-1}`, `{"expected_revision":9223372036854775807}`, `{"expected_revision":0,"aliases":{"bad alias":[1]}}`, `{"expected_revision":0,"padding":"` + strings.Repeat("x", 65536) + `"}`} {
		request("PUT", path, adminToken, invalid, "", 400)
	}
	initial := request("GET", path, memberToken, "", "", 200)
	if !strings.Contains(initial.Body.String(), `"revision":0`) {
		t.Fatal(initial.Body.String())
	}
	saved := request("PUT", path, adminToken, body, "", 200)
	if saved.Header().Get("Cache-Control") != "no-store" || !strings.Contains(saved.Body.String(), `"revision":1`) {
		t.Fatal(saved.Header(), saved.Body.String())
	}
	read := request("GET", path, memberToken, "", "", 200)
	if read.Body.String() != saved.Body.String() {
		t.Fatal("configuration read differs", read.Body.String())
	}
	request("PUT", path, adminToken, body, "", 409)
	var actor int64
	if err := s.DB.QueryRow(`SELECT actor FROM platform_owner_routing_history WHERE project_id=1 AND revision=1`).Scan(&actor); err != nil || actor != admin.ID {
		t.Fatal("actor spoofed", actor, err)
	}
	if err := s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", path, memberToken, "", "", 404)
	request("PUT", path, adminToken, strings.Replace(body, `"expected_revision":0`, `"expected_revision":1`, 1), "", 404)
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_owner_routing_history`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected write changed history", count, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_project_members WHERE user_id=?`, member.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("routing granted access", count, err)
	}
}
