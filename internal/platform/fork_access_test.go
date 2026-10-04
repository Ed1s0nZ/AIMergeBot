package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type forkAccessRepo struct{ runRepo }

func (r forkAccessRepo) Snapshot(ctx context.Context, p, i int) (Snapshot, error) {
	snap, err := r.runRepo.Snapshot(ctx, p, i)
	snap.SourceProjectID = 2
	return snap, err
}

func TestForkAuditRequiresExplicitSourceReadAccess(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "operator", admin.ID); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
	if _, _, err := s.EnqueueUser(ctx, snap, member.ID, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("target operator read ungranted source", err)
	}
	runner := &Runner{Store: s, Repository: forkAccessRepo{}}
	if _, _, err := runner.Submit(ctx, 1, 99, member.ID, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("metadata fetch bypassed source access", err)
	}
	id, _, err := s.Enqueue(ctx, snap, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s}).Register(router)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1"+path, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := get("/runs/1"); w.Code != 404 {
		t.Fatal("fork detail leaked source content", w.Body.String())
	}
	w := get("/runs")
	var list struct {
		Items []Run `json:"items"`
		Total int   `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if w.Code != 200 || list.Total != 0 || len(list.Items) != 0 {
		t.Fatal("fork list leaked source result/count", w.Body.String())
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 2, Name: "read-only-source", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if w := get("/runs/1"); w.Code != 200 {
		t.Fatal("explicit source read did not permit detail", w.Body.String())
	}
	if _, _, err = s.EnqueueUser(ctx, snap, member.ID, false); err != nil {
		t.Fatal("authorized fork dedupe blocked", err)
	}
	// A distinct user request is cancelled when source read permission is removed.
	snap.MRIID = 2
	requested, _, err := runner.Submit(ctx, 1, 2, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 2, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, requested)
	if err != nil || run.Status != "cancelled" {
		t.Fatal("source revocation retained dependent run", run.Status, err)
	}
	system, err := s.Run(ctx, id)
	if err != nil || system.Status != "pending" {
		t.Fatal("member revocation cancelled system-owned task", err)
	}
	if w := get("/runs/1"); w.Code != 404 {
		t.Fatal("source revocation retained detail access")
	}
	// A trusted queue insertion with stale user provenance still cannot execute.
	snap.MRIID = 3
	stale, _, err := s.Enqueue(ctx, snap, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE platform_runs SET status='running' WHERE id=?`, stale); err != nil {
		t.Fatal(err)
	}
	a := &transientAuditor{}
	r := &Runner{Store: s, Repository: runRepo{}, Auditor: a, active: map[int64]context.CancelFunc{}}
	r.execute(ctx, stale)
	run, err = s.Run(ctx, stale)
	if err != nil || run.Status != "cancelled" || a.calls != 0 {
		t.Fatal("worker accessed revoked fork", run.Status, a.calls, err)
	}
}
