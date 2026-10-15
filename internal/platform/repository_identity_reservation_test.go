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

func TestRepositoryIdentityAllocatorReservesHistoricalProjectIDs(t *testing.T) {
	for _, mode := range []string{"target", "source", "context"} {
		t.Run(mode, func(t *testing.T) {
			s, admin, _ := accessFixture(t)
			ctx := context.Background()
			old := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
			if mode == "target" {
				old.ProjectID = 3
				old.SourceProjectID = 3
			}
			if mode == "source" {
				old.SourceProjectID = 3
			}
			id, _, err := s.Enqueue(ctx, old, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Summary: "OLD-PRIVATE-EVIDENCE"}, nil); err != nil {
				t.Fatal(err)
			}
			if mode == "context" {
				policy := AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 3, SHA: old.HeadSHA}}}
				raw, _ := json.Marshal(policy)
				if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json=? WHERE id=?`, string(raw), id); err != nil {
					t.Fatal(err)
				}
				if _, err = s.DB.Exec(`INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,?,?)`, id, 3, old.HeadSHA); err != nil {
					t.Fatal(err)
				}
			}
			v := repositoryProfile(t, s, admin.ID, "github")
			receipt, err := s.CreateRepositoryProject(ctx, admin.ID, repositoryProjectInput(v, "77777777-7777-4777-8777-777777777777"))
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Project.ID != 4 {
				t.Errorf("historical project id reused: got %d want 4", receipt.Project.ID)
			}
			reader, err := s.CreateUser(ctx, "new-reader", "synthetic-reader-password", "member")
			if err != nil {
				t.Fatal(err)
			}
			for _, project := range []int{1, receipt.Project.ID} {
				if err = s.SetProjectMember(ctx, project, reader.ID, "viewer", admin.ID); err != nil {
					t.Fatal(err)
				}
			}
			_, token, err := s.Login(ctx, "new-reader", "synthetic-reader-password")
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			(&HTTP{Store: s}).Register(router)
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%d", id), nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if (response.Code != 403 && response.Code != 404) || strings.Contains(response.Body.String(), "OLD-PRIVATE-EVIDENCE") {
				t.Fatal("new repository membership exposed historical report", response.Code)
			}
		})
	}
}
