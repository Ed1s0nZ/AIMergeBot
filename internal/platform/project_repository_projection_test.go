package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProjectRepositoryProjectionPermissionsAndIdentity(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	profile := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(profile, "github", "https://api.example")); err != nil {
		t.Fatal(err)
	}
	forged := member
	forged.Role = "admin"
	items, err := s.ProjectsForUser(ctx, forged)
	if err != nil || len(items) != 0 {
		t.Fatal("stale caller role exposed projects", items, err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	items, err = s.ProjectsForUser(ctx, member)
	if err != nil || len(items) != 1 || items[0].Repository == nil || items[0].Repository.Provider != "github" || items[0].Repository.RemoteID != 7 || items[0].RepositoryExecutionAvailable == nil || *items[0].RepositoryExecutionAvailable || items[0].AccessRole != "viewer" {
		t.Fatal(items, err)
	}
	all, err := s.ProjectsForUser(ctx, admin)
	if err != nil || len(all) != 2 {
		t.Fatal(all, err)
	}
	for _, p := range all {
		if p.ID == 2 && (p.Repository != nil || p.RepositoryExecutionAvailable != nil) {
			t.Fatal("legacy identity changed", p)
		}
	}
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	request := func(want int) string {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/projects", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatal(res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), "fixture-private-token") {
			t.Fatal("credential exposed")
		}
		return res.Body.String()
	}
	body := request(200)
	if !strings.Contains(body, `"repository_execution_available":false`) || !strings.Contains(body, `"provider":"github"`) {
		t.Fatal(body)
	}
	if _, err = s.DB.Exec(`UPDATE platform_project_repositories SET binding_json='corrupt' WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	request(409)
	if err = s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	body = request(200)
	var list struct {
		Items []Project `json:"items"`
	}
	if err = json.Unmarshal([]byte(body), &list); err != nil || len(list.Items) != 0 {
		t.Fatal("revoked metadata exposed", body, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ProjectsForUser(ctx, forged); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	request(401)
}
