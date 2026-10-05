package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func accessFixture(t *testing.T) (*Store, User, User) {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	admin, err := s.CreateUser(ctx, "admin", "admin-long-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "member", "member-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err = s.SaveProject(ctx, Project{ID: id, Name: fmt.Sprint("project-", id), Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	return s, admin, member
}

func TestProjectAccessMigrationIsOneTimeAndPreservesRevocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.CreateUser(ctx, "admin", "admin-long-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "member", "member-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err = s.SaveProject(ctx, Project{ID: id, Name: fmt.Sprint(id), Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	// Remove only new ACL tables to produce an actual pre-ACL database layout.
	for _, q := range []string{`DROP TABLE platform_project_members`, `DROP TABLE platform_access_migrations`} {
		if _, err = s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.ProjectsForUser(ctx, member)
	if err != nil || len(items) != 2 || items[0].AccessRole != "operator" {
		t.Fatal("migration lost prior access", items, err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err = s.ProjectsForUser(ctx, member)
	if err != nil || len(items) != 1 || items[0].AccessRole != "viewer" {
		t.Fatal("restart regenerated revoked access", items, err)
	}
	newMember, err := s.CreateUser(ctx, "new-member", "new-member-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if items, err = s.ProjectsForUser(ctx, newMember); err != nil || len(items) != 0 {
		t.Fatal("new member auto-granted", items, err)
	}
	if err = s.SaveProject(ctx, Project{ID: 3, Name: "new-project", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = projectRole(ctx, s.DB, 3, member.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("new project auto-granted", err)
	}
}

func TestProjectHTTPIsolationAndRoleTransitions(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	var runs []int64
	for _, project := range []int{1, 2} {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: project, SourceProjectID: project, MRIID: 99, BaseSHA: "base", HeadSHA: "head"}, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		s.Claim(ctx)
		if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f1", File: "a.any", Line: 1, Severity: "high", Title: "fixture"}}}, nil); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, id)
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
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Runner: &Runner{Store: s, Repository: runRepo{}}}).Register(router)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	check := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := request(method, path, body, token)
		if w.Code != status {
			t.Fatalf("%s%s: got%d want%d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	check("GET", "/api/v1/runs/1", "", memberToken, 404)
	check("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":1}`, memberToken, 404)
	for _, path := range []string{"/api/v1/projects", "/api/v1/runs"} {
		w := check("GET", path, "", memberToken, 200)
		if !strings.Contains(w.Body.String(), `"items":[]`) {
			t.Fatal("ungranted member sees items", w.Body.String())
		}
	}
	check("GET", "/api/v1/projects/1/members", "", memberToken, 403)
	check("PUT", fmt.Sprintf("/api/v1/projects/1/members/%d", member.ID), `{"role":"operator"}`, memberToken, 403)
	check("PUT", fmt.Sprintf("/api/v1/projects/1/members/%d", member.ID), `{"role":"viewer"}`, adminToken, 204)
	w := check("GET", "/api/v1/runs?project_id=2", "", memberToken, 200)
	if !strings.Contains(w.Body.String(), `"total":0`) {
		t.Fatal("inaccessible count leaked")
	}
	w = check("GET", "/api/v1/runs", "", memberToken, 200)
	var list struct {
		Items []Run `json:"items"`
		Total int   `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ProjectID != 1 {
		t.Fatal("cross-project list leak", w.Body.String())
	}
	check("GET", fmt.Sprintf("/api/v1/runs/%d", runs[1]), "", memberToken, 404)
	w = check("GET", fmt.Sprintf("/api/v1/runs/%d", runs[0]), "", memberToken, 200)
	var detail struct {
		Permissions ProjectPermissions `json:"permissions"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Permissions.CanSubmit || detail.Permissions.CanReview || detail.Permissions.Role != "viewer" {
		t.Fatal("viewer permissions wrong")
	}
	reviewPath := fmt.Sprintf("/api/v1/runs/%d/findings/f1/review", runs[0])
	check("PUT", reviewPath, `{"status":"accepted","reason":"fixture","expected_revision":0}`, memberToken, 403)
	check("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":1}`, memberToken, 403)
	if err = s.SetProjectMember(ctx, 1, member.ID, "reviewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	check("PUT", reviewPath, `{"status":"accepted","reason":"fixture","expected_revision":0}`, memberToken, 204)
	check("PUT", reviewPath, `{"status":"accepted"}`, memberToken, 400)
	check("PUT", reviewPath, `{"status":"fixed","expected_revision":0}`, memberToken, 409)
	check("PUT", reviewPath, `{"status":"fixed","expected_revision":1}`, memberToken, 204)
	check("POST", "/api/v1/runs/1/cancel", `{}`, memberToken, 403)
	if err = s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	w = check("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":1}`, memberToken, 202)
	var submitted struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &submitted)
	check("POST", fmt.Sprintf("/api/v1/runs/%d/cancel", submitted.ID), `{}`, memberToken, 204)
	w = check("POST", "/api/v1/runs", `{"project_id":1,"mr_iid":2}`, memberToken, 202)
	json.Unmarshal(w.Body.Bytes(), &submitted)
	if err = s.SaveProject(ctx, Project{ID: 1, Name: "project-1", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	w = check("GET", fmt.Sprintf("/api/v1/runs/%d", submitted.ID), "", memberToken, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil || detail.Permissions.CanSubmit || !detail.Permissions.CanCancel {
		t.Fatal("disabled project capabilities wrong", w.Body.String())
	}
	check("DELETE", fmt.Sprintf("/api/v1/projects/1/members/%d", member.ID), "", adminToken, 204)
	run, err := s.Run(ctx, submitted.ID)
	if err != nil || run.Status != "cancelled" {
		t.Fatal("revocation failed to cancel pending task", err, run.Status)
	}
	check("GET", fmt.Sprintf("/api/v1/runs/%d", submitted.ID), "", memberToken, 404)
	check("GET", fmt.Sprintf("/api/v1/runs/%d", submitted.ID), "", adminToken, 200)
	if err = s.UpdateUser(ctx, member.ID, "member", true, ""); err != nil {
		t.Fatal(err)
	}
	check("GET", "/api/v1/projects", "", memberToken, 401)
}

type accessRaceRepo struct {
	runRepo
	beforeReturn func()
}

func (r accessRaceRepo) Snapshot(ctx context.Context, p, i int) (Snapshot, error) {
	r.beforeReturn()
	return r.runRepo.Snapshot(ctx, p, i)
}

func TestUserWriteRechecksPermissionsAfterMetadataFetch(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Store: s, Repository: accessRaceRepo{beforeReturn: func() {
		if err := s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
			t.Fatal(err)
		}
	}}}
	if _, _, err := r.Submit(ctx, 1, 1, member.ID, false); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("stale access enqueued", err)
	}
	var count int
	s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count)
	if count != 0 {
		t.Fatal("rejected write persisted run")
	}
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	id, _, err := s.EnqueueUser(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelUser(ctx, id, member.ID); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("stale cancellation access", err)
	}
	if err = s.UpdateUser(ctx, member.ID, "member", true, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnqueueUser(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 2, BaseSHA: "base", HeadSHA: "head"}, member.ID, false); !errors.Is(err, ErrCredentials) {
		t.Fatal("disabled user enqueued", err)
	}
}
