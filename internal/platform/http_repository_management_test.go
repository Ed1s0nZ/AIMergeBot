package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	legacy "pr_agent/internal"
)

// Hash rows rather than printing any integration/session secret on failure.
func repositoryManagementDBFingerprint(t *testing.T, s *Store) string {
	t.Helper()
	rows, err := s.DB.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	for _, name := range names {
		rows, err := s.DB.Query(`SELECT * FROM "` + strings.ReplaceAll(name, `"`, `""`) + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		encoded := []string{}
		for rows.Next() {
			values := make([]any, len(columns))
			args := make([]any, len(columns))
			for i := range values {
				args[i] = &values[i]
			}
			if err = rows.Scan(args...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			raw, e := json.Marshal(values)
			if e != nil {
				rows.Close()
				t.Fatal(e)
			}
			encoded = append(encoded, string(raw))
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(encoded)
		raw, err := json.Marshal(struct {
			Name string
			Rows []string
		}{name, encoded})
		if err != nil {
			t.Fatal(err)
		}
		hash.Write(raw)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type repositoryUnavailableBody struct{ reads int }

func (b *repositoryUnavailableBody) Read([]byte) (int, error) {
	b.reads++
	return 0, fmt.Errorf("unexpected request body read")
}
func (*repositoryUnavailableBody) Close() error { return nil }

func TestRepositoryManagementHTTPReadOnlyAndNoWriteSideEffects(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	if err := s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	profile := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	binding, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, RepositoryBinding{Provider: "github", APIOrigin: "https://api.example", RemoteID: 7, FullName: "org/repo", IntegrationID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "config.yaml")
	settings, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
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
			t.Fatalf("%s %s got %d want %d", method, path, res.Code, want)
		}
		if res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("response cacheable")
		}
		if strings.Contains(res.Body.String(), "fixture-private-token") {
			t.Fatal("secret in response")
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
		t.Fatal("missing session")
		return ""
	}
	adminToken, memberToken := login("admin"), login("member")
	beforeDB := repositoryManagementDBFingerprint(t, s)
	beforeFile, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	beforeConfig := settings.Snapshot()
	path := "/api/v1/projects/1/repository-binding"
	request("GET", path, "", "", "", 401)
	request("GET", "/api/v1/projects/2/repository-binding", memberToken, "", "", 404)
	request("GET", "/api/v1/projects/999/repository-binding", adminToken, "", "", 404)
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		request("GET", "/api/v1/projects/"+id+"/repository-binding", adminToken, "", "", 400)
	}
	for _, token := range []string{adminToken, memberToken} {
		got := request("GET", path, token, "", "", 200)
		var decoded RepositoryBinding
		if json.Unmarshal(got.Body.Bytes(), &decoded) != nil || decoded != binding {
			t.Fatal("saved binding changed in read projection")
		}
		projects := request("GET", "/api/v1/projects", token, "", "", 200)
		var projection struct {
			Items      []Project                  `json:"items"`
			Management repositoryManagementStatus `json:"repository_management"`
		}
		if json.Unmarshal(projects.Body.Bytes(), &projection) != nil || projection.Management.Available || projection.Management.Reason != "native_audit_not_ready" || len(projection.Items) == 0 {
			t.Fatal("incorrect availability projection")
		}
	}
	unbound := request("GET", "/api/v1/projects/2/repository-binding", adminToken, "", "", 200)
	var legacy RepositoryBinding
	if json.Unmarshal(unbound.Body.Bytes(), &legacy) != nil || legacy.Revision != 0 {
		t.Fatal("legacy binding projection changed")
	}
	for _, write := range []struct{ method, path string }{{"POST", "/api/v1/repository-projects"}, {"PATCH", path}, {"PATCH", "/api/v1/projects/2/repository-binding"}, {"PATCH", "/api/v1/projects/999/repository-binding"}} {
		request(write.method, write.path, "", `{}`, "", 401)
		request(write.method, write.path, memberToken, `{}`, "", 403)
		request(write.method, write.path, adminToken, `{}`, "https://attacker.example", 403)
		for _, body := range []string{`{}`, `null`, `[]`, `{"provider":"github","expected_revision":1}`, strings.Repeat("x", 16384)} {
			res := request(write.method, write.path, adminToken, body, "", 501)
			var status struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(res.Body.Bytes(), &status) != nil || status.Code != "repository_management_unavailable" {
				t.Fatal("missing explicit unavailable code")
			}
		}
		unread := &repositoryUnavailableBody{}
		req := httptest.NewRequest(write.method, write.path, nil)
		req.Body = unread
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: adminToken})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != 501 || unread.reads != 0 {
			t.Fatal("unavailable write parsed request body")
		}
	}
	if repositoryManagementDBFingerprint(t, s) != beforeDB {
		t.Fatal("unavailable write mutated database")
	}
	afterFile, err := os.ReadFile(target)
	if err != nil || string(afterFile) != string(beforeFile) {
		t.Fatal("unavailable write changed configuration file")
	}
	beforeJSON, _ := json.Marshal(beforeConfig)
	afterJSON, _ := json.Marshal(settings.Snapshot())
	if string(beforeJSON) != string(afterJSON) || repo.snapshots.Load() != 0 {
		t.Fatal("unavailable write published config or read repository")
	}
	request("POST", "/api/v1/runs", adminToken, `{"project_id":1,"mr_iid":1}`, "", 503)
	if repo.snapshots.Load() != 0 {
		t.Fatal("read-only binding unexpectedly enabled legacy execution")
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	request("GET", path, memberToken, "", "", 404)
}

func TestRepositoryManagementLegacyRoutesCannotCreateNativeBinding(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	_, token, err := s.Login(ctx, "admin", "admin-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	body := `{"id":3,"name":"legacy repository","enabled":true,"internal_project":true,"repository":{"provider":"github","api_origin":"https://api.example","remote_id":9,"full_name":"org/repo","integration_id":1}}`
	for _, method := range []string{"POST", "PATCH"} {
		path := "/api/v1/projects"
		if method == "PATCH" {
			path += "/3"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("legacy project operation got %d", res.Code)
		}
		binding, err := s.RepositoryBinding(ctx, 3, admin.ID)
		if err != nil || binding.Revision != 0 {
			t.Fatal("legacy route created a native binding")
		}
		identity, err := readRepositoryIdentity(ctx, s.DB, 3)
		if err != nil || identity.Kind != "legacy" {
			t.Fatal("legacy route allocated native identity")
		}
	}
	before := repositoryManagementDBFingerprint(t, s)
	_, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 3, Name: "must roll back"}, {ID: 44, Name: "native injection", InternalProject: true}})
	if err == nil || repositoryManagementDBFingerprint(t, s) != before {
		t.Fatal("configured-project import created an unsupported native identity or failed to roll back")
	}
}
