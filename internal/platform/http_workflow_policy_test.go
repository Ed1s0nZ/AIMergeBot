package platform

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkflowPolicyHTTPRevisionContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	h := &HTTP{Store: s}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user", User{ID: 1, Role: "admin"}) })
	router.GET("/projects/:id/workflow-policy", h.workflowPolicy)
	router.PUT("/projects/:id/workflow-policy", h.saveWorkflowPolicy)
	request := func(method, path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	request("GET", "/projects/1/workflow-policy", "", 200)
	request("PUT", "/projects/1/workflow-policy", `{"focus":["ssrf"]}`, 400)
	request("PUT", "/projects/1/workflow-policy", `{"expected_revision":0,"focus":["ssrf"],"excluded_extensions":[".svg"]}`, 200)
	request("PUT", "/projects/1/workflow-policy", `{"expected_revision":0,"focus":[]}`, 409)
	request("PUT", "/projects/1/workflow-policy", `{"expected_revision":1,"focus":["arbitrary prompt"]}`, 400)
	request("PUT", "/projects/1/workflow-policy", `{"expected_revision":1,"focus":[],"padding":"`+strings.Repeat("x", 17000)+`"}`, 400)
}
