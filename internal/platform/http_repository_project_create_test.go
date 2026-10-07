package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRepositoryProjectHTTPBootstrapAndSyncReplay(t *testing.T) {
	for _, syncFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(syncFailure), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			for _, role := range []string{"admin", "member"} {
				if _, err := s.CreateUser(ctx, role, role+"-long-password", role); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(t.TempDir(), "config.yaml")
			settings, err := OpenSettings(target, "../../config.example.yaml")
			if err != nil {
				t.Fatal(err)
			}
			repo := &bindingGuardRepository{}
			router := gin.New()
			(&HTTP{Store: s, Settings: settings, Runner: &Runner{Store: s, Repository: repo}}).Register(router)
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
				if strings.Contains(res.Body.String(), "fixture-private-token") || strings.Contains(res.Body.String(), target) {
					t.Fatal("private data leaked", res.Body.String())
				}
				if res.Header().Get("Cache-Control") != "no-store" {
					t.Fatal(res.Header())
				}
				return res
			}
			login := func(name string) string {
				res := request("POST", "/api/v1/auth/login", "", fmt.Sprintf(`{"username":%q,"password":%q}`, name, name+"-long-password"), "", 200)
				for _, cookie := range res.Result().Cookies() {
					if cookie.Name == sessionCookie {
						return cookie.Value
					}
				}
				t.Fatal("no session")
				return ""
			}
			adminToken, memberToken := login("admin"), login("member")
			profileBody := `{"expected_revision":0,"name":"GitHub credentials","kind":"github","enabled":true,"project_ids":[],"frequency":"instant","credentials":{"endpoint":"https://api.example","token":"fixture-private-token"}}`
			request("POST", "/api/v1/integrations", memberToken, profileBody, "", 403)
			profileRes := request("POST", "/api/v1/integrations", adminToken, profileBody, "", 200)
			var profile Integration
			if err = json.Unmarshal(profileRes.Body.Bytes(), &profile); err != nil || profile.Revision != 1 || len(profile.ProjectIDs) != 0 || !strings.Contains(profileRes.Body.String(), `"project_ids":[]`) || !strings.Contains(profileRes.Body.String(), `"events":[]`) {
				t.Fatal(profile, err)
			}
			input := repositoryProjectInput(profile, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			raw, _ := json.Marshal(input)
			body := string(raw)
			path := "/api/v1/repository-projects"
			request("POST", path, "", body, "", 401)
			request("POST", path, memberToken, body, "", 403)
			request("POST", path, adminToken, body, "https://attacker.example", 403)
			for _, invalid := range []string{
				`{}`, `null`, `[]`, body + ` {}`, body + ` null`, body + ` trailing`,
				strings.Replace(body, `"expected_integration_revision":1,`, "", 1),
				strings.Replace(body, `"expected_integration_revision":1`, `"expected_integration_revision":null`, 1),
				strings.Replace(body, `"expected_integration_revision":1`, `"expected_integration_revision":0`, 1),
				strings.Replace(body, `"expected_integration_revision":1`, `"expected_integration_revision":9223372036854775807`, 1),
				strings.TrimSuffix(body, "}") + `,"id":123}`,
				strings.TrimSuffix(body, "}") + `,"actor":123}`,
				strings.TrimSuffix(body, "}") + `,"token":"private-input"}`,
				strings.TrimSuffix(body, "}") + `,"padding":"` + strings.Repeat("x", 8192) + `"}`,
			} {
				request("POST", path, adminToken, invalid, "", 400)
			}
			request("POST", path, adminToken, strings.Replace(body, "https://api.example", "https://wrong.example", 1), "", 409)
			request("POST", path, adminToken, strings.Replace(body, fmt.Sprintf(`"integration_id":%d`, profile.ID), `"integration_id":999`, 1), "", 404)
			if syncFailure {
				if err = os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					res := request("POST", path, adminToken, body, "", 500)
					if !strings.Contains(res.Body.String(), "project_config_sync_pending") {
						t.Fatal(res.Body.String())
					}
				}
				if err = os.Remove(target); err != nil {
					t.Fatal(err)
				}
			}
			want := 201
			if syncFailure {
				want = 200
			}
			created := request("POST", path, adminToken, body, "", want)
			var receipt RepositoryProjectReceipt
			if err = json.Unmarshal(created.Body.Bytes(), &receipt); err != nil || receipt.Project.ID != 1 || receipt.Binding.Revision != 1 || receipt.IntegrationRevision != 2 || receipt.Replayed != syncFailure {
				t.Fatal(receipt, err)
			}
			repeated := request("POST", path, adminToken, body, "", 200)
			if err = json.Unmarshal(repeated.Body.Bytes(), &receipt); err != nil || !receipt.Replayed || receipt.Project.ID != 1 {
				t.Fatal(receipt, err)
			}
			request("POST", path, adminToken, strings.Replace(body, `"name":"new repository"`, `"name":"other"`, 1), "", 409)
			request("POST", "/api/v1/projects", adminToken, `{"id":1,"name":"collision","enabled":true}`, "", 409)
			unavailable := request("POST", "/api/v1/runs", adminToken, `{"project_id":1,"mr_iid":1}`, "", 503)
			if !strings.Contains(unavailable.Body.String(), "repository_unavailable") || repo.snapshots.Load() != 0 {
				t.Fatal("legacy read occurred", unavailable.Body.String())
			}
			reopened, err := OpenSettings(target, "../../config.example.yaml")
			if err != nil || len(reopened.Snapshot().Projects) != 1 || !reopened.Snapshot().Projects[0].InternalProject {
				t.Fatal("namespace not persisted", err)
			}
			for _, table := range []string{"platform_projects", "platform_repository_project_receipts", "platform_repository_binding_history"} {
				var count int
				if err = s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
					t.Fatal("duplicate creation", table, count, err)
				}
			}
		})
	}
}
