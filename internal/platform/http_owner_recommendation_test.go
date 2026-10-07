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

func TestOwnerRecommendationHTTPRequiresFullSnapshotSession(t *testing.T) {
	s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
	ctx := context.Background()
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{Aliases: map[string][]int64{"@owner": {admin.ID}}}); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s, Runner: &Runner{Repository: repo}}).Register(router)
	path := fmt.Sprintf("/api/v1/runs/%d/findings/f/owners", run)
	request := func(path, session string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		if session != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatal(res.Code, want, res.Body.String())
		}
		return res
	}
	request(path, "", 401)
	res := request(path, token, 200)
	if res.Header().Get("Cache-Control") != "no-store" || !strings.Contains(res.Body.String(), `"source":"codeowners"`) || !strings.Contains(res.Body.String(), `"repository_id":3`) || strings.Contains(res.Body.String(), `"audit_policy"`) || strings.Contains(res.Body.String(), `"rules":{`) {
		t.Fatal(res.Header(), res.Body.String())
	}
	repo.reads = nil
	request(fmt.Sprintf("/api/v1/runs/%d/findings/missing/owners", run), token, 404)
	if len(repo.reads) != 0 {
		t.Fatal("missing finding read repository")
	}
	for _, project := range []int{1, 2, 3} {
		if err := s.SetProjectMember(ctx, project, viewer.ID, "", admin.ID); err != nil {
			t.Fatal(err)
		}
		request(path, token, 404)
		if len(repo.reads) != 0 {
			t.Fatal("unauthorized repository read", project)
		}
		if err := s.SetProjectMember(ctx, project, viewer.ID, "viewer", admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	request(path, token, 401)
}
