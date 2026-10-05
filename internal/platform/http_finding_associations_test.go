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

func TestAssociationHTTPRequiresBothSnapshotPermissionsAndRejectsConflicts(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "reviewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	links := []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("a", 40)}}
	if err := s.SaveContextRepositories(ctx, 1, links, admin.ID); err != nil {
		t.Fatal(err)
	}
	current, old := associationFixture()
	old.AuditPolicy = &AuditPolicy{ContextRepositories: links}
	current.AuditPolicy = old.AuditPolicy
	persistAssociationRun(t, s, old)
	current = persistAssociationRun(t, s, current)
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s}).Register(router)
	base := fmt.Sprintf("/api/v1/runs/%d/associations", current.ID)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	if out := request("GET", base, ""); out.Code != 404 {
		t.Fatal("context permission bypass", out.Code)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	out := request("GET", base, "")
	var list FindingAssociations
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Fatal("HTTP candidate missing", out.Code, out.Body.String())
	}
	path := base + "/" + list.Items[0].ID
	if out = request("PUT", path, `{"decision":"confirmed","reason":" "}`); out.Code != 400 {
		t.Fatal("invalid decision status", out.Code)
	}
	if out = request("PUT", path, `{"decision":"confirmed","reason":"private related evidence inspected","expected_revision":0}`); out.Code != 200 {
		t.Fatal("HTTP confirmation failed", out.Code, out.Body.String())
	}
	if out = request("PUT", path, `{"decision":"rejected","reason":"stale decision","expected_revision":0}`); out.Code != 409 {
		t.Fatal("stale HTTP decision overwrote", out.Code)
	}
	if out = request("PUT", base+"/forged", `{"decision":"confirmed","reason":"fake"}`); out.Code != 409 {
		t.Fatal("forged candidate accepted", out.Code)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PUT"} {
		target, body := base, ""
		if method == "PUT" {
			target = path
			body = `{"decision":"rejected","reason":"revoked"}`
		}
		out = request(method, target, body)
		if out.Code != 404 || strings.Contains(out.Body.String(), "private related") {
			t.Fatal("revoked context leaked decision", method, out.Code)
		}
	}
}
