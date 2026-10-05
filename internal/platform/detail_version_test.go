package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDetailVersionTracksReportReviewsAndComments(t *testing.T) {
	s, id := revisionFixture(t)
	h := &HTTP{Store: s}
	ctx := context.Background()
	read := func() string {
		v, err := h.detailStatus(ctx, id, 1, "admin")
		if err != nil {
			t.Fatal(err)
		}
		return v.Version
	}
	version := read()
	if read() != version {
		t.Fatal("stable state changed version")
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json=replace(result_json,'"summary":""','"summary":"aa"') WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if read() == version {
		t.Fatal("report mutation missed")
	}
	version = read()
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json=replace(result_json,'"summary":"aa"','"summary":"bb"') WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if read() == version {
		t.Fatal("same-length JSON mutation missed")
	}
	version = read()
	if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "accepted"})); err != nil {
		t.Fatal(err)
	}
	if read() == version {
		t.Fatal("review mutation missed")
	}
	version = read()
	if _, err := s.DB.Exec(`UPDATE platform_comment_delivery SET state='unknown' WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if read() == version {
		t.Fatal("comment mutation missed")
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	if read() == "" {
		t.Fatal("migration lost revision state")
	}
}

func TestDetailStatusDoesNotTransmitSourceAndRechecksPermissions(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Summary: strings.Repeat("private-source", 10000)}, []ToolTrace{{Name: "read_file", Output: strings.Repeat("secret-code", 10000)}}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "member", "member-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	status := request(fmt.Sprintf("/api/v1/runs/%d/status", id))
	if status.Code != 200 || status.Body.Len() > 400 || strings.Contains(status.Body.String(), "private-source") || strings.Contains(status.Body.String(), "secret-code") {
		t.Fatalf("status payload: %d %s", status.Code, status.Body.String())
	}
	var small runDetailStatus
	if err = json.Unmarshal(status.Body.Bytes(), &small); err != nil || small.Version == "" {
		t.Fatal("missing version", err)
	}
	full := request(fmt.Sprintf("/api/v1/runs/%d", id))
	if full.Code != 200 || full.Body.Len() < 200000 {
		t.Fatalf("full report unavailable %d", full.Code)
	}
	var detail struct {
		Version string `json:"detail_version"`
	}
	json.Unmarshal(full.Body.Bytes(), &detail)
	if detail.Version != small.Version {
		t.Fatal("unchanged detail/version mismatch")
	}
	export := request(fmt.Sprintf("/api/v1/runs/%d/sarif", id))
	if export.Code != 200 || export.Header().Get("Content-Type") != "application/sarif+json" || export.Header().Get("Cache-Control") != "no-store" || !strings.Contains(export.Header().Get("Content-Disposition"), ".sarif") {
		t.Fatalf("export response: %d %v", export.Code, export.Header())
	}
	var exported map[string]any
	if err = json.Unmarshal(export.Body.Bytes(), &exported); err != nil || exported["version"] != "2.1.0" {
		t.Fatal("invalid export", err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	revoked := request(fmt.Sprintf("/api/v1/runs/%d/status", id))
	if revoked.Code == 200 || strings.Contains(revoked.Body.String(), small.Version) {
		t.Fatal("revoked user received version")
	}
	blockedExport := request(fmt.Sprintf("/api/v1/runs/%d/sarif", id))
	if blockedExport.Code == 200 || strings.Contains(blockedExport.Body.String(), "private-source") {
		t.Fatal("revoked user could export")
	}
	t.Logf("synthetic unchanged payload: full=%d bytes status=%d bytes", full.Body.Len(), status.Body.Len())
}

func TestDetailVersionChangesWhenRetryDelayExpiresWithoutDatabaseWrite(t *testing.T) {
	s, id := revisionFixture(t)
	ctx := context.Background()
	h := &HTTP{Store: s}
	retry := time.Now().Add(150 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	if _, err := s.DB.Exec(`UPDATE platform_runs SET status='pending',retry_at=? WHERE id=?`, retry, id); err != nil {
		t.Fatal(err)
	}
	first, err := h.detailStatus(ctx, id, 1, "admin")
	if err != nil || first.Wait == nil || first.Wait.Reason != "retry_delay" {
		t.Fatalf("missing retry delay: %+v %v", first, err)
	}
	time.Sleep(200 * time.Millisecond)
	second, err := h.detailStatus(ctx, id, 1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if second.Version == first.Version || (second.Wait != nil && second.Wait.Reason == "retry_delay") {
		t.Fatal("time-only queue change was missed")
	}
}
