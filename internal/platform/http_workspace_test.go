package platform

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceFindingPagingAndSnapshotACL(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "reader", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2, 3} {
		if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(?,'fixture',1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	grant := func(id int) {
		t.Helper()
		if _, err := s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,'viewer')`, id, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	grant(1)
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"findings":[{"id":"f1","severity":"high","type":"injection","title":"first","file":"a.go","line":4,"evidence":"PRIVATE"},{"id":"f2","severity":"low","type":"auth","title":"second","file":"b.py","line":9}],"coverage_notes":[]}`
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='incomplete',result_json=?,trace_json='invalid-private-trace' WHERE id=?`, payload, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,3,'context')`, id); err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		where, args := workspaceACL(u)
		items, total, err := s.workspaceFindings(ctx, where, args, 1, 1)
		if err != nil || total != want {
			t.Fatalf("total=%d want=%d err=%v", total, want, err)
		}
		if want > 0 {
			if len(items) != 1 || items[0].FindingID != "f1" || items[0].ReviewStatus != "pending" {
				t.Fatalf("items %+v", items)
			}
			body, _ := json.Marshal(items)
			if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "trace") {
				t.Fatal("private payload leaked")
			}
		}
	}
	check(0)
	grant(2)
	check(0)
	grant(3)
	check(2)
	where, args := workspaceACL(u)
	items, total, err := s.workspaceFindings(ctx, where, args, 2, 1)
	if err != nil || total != 2 || len(items) != 1 || items[0].FindingID != "f2" {
		t.Fatalf("page2: %+v %d %v", items, total, err)
	}
	_, total, err = s.workspaceFindings(ctx, where+" AND f.severity=? AND f.kind=?", append(args, "high", "auth"), 1, 20)
	if err != nil || total != 0 {
		t.Fatal("filters matched different findings", total, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=3 AND user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	check(0)
}

func TestWorkspaceHTTPTaskKindsAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := testStore(t)
	ctx := context.Background()
	h := &HTTP{Store: s}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user", User{ID: 1, Role: "admin"}) })
	router.GET("/findings", h.workspaceFindings)
	router.GET("/tasks", h.workspaceTasks)
	for i, status := range []string{"failed", "incomplete", "succeeded", "running"} {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: i + 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		result := `{"findings":[],"coverage_notes":[]}`
		if status == "succeeded" {
			result = `{"findings":[{"id":"pending","severity":"high"}],"coverage_notes":[]}`
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status=?,result_json=? WHERE id=?`, status, result, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		path        string
		code, total int
	}{{"/tasks?kind=failed", 200, 1}, {"/tasks?kind=incomplete", 200, 1}, {"/tasks?kind=pending_review", 200, 1}, {"/tasks?kind=unknown", 400, 0}, {"/tasks?kind=failed&project_id=x", 400, 0}, {"/findings?project_id=-1", 400, 0}, {"/findings?severity=" + strings.Repeat("x", 81), 400, 0}, {"/findings?page=2&size=1", 200, 1}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if tc.code == 200 {
			var body struct {
				Total int `json:"total"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Total != tc.total {
				t.Fatalf("%s: %s %v", tc.path, w.Body.String(), err)
			}
		}
	}
}
