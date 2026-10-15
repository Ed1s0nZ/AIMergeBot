package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCredentialReloadHTTPContract(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{})
	reloadAdministrator(t, s)
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "member", "member-long-password", "member"); err != nil {
		t.Fatal(err)
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
	(&HTTP{Store: s, Settings: svc}).Register(router)
	request := func(method, path, token, body, origin string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if token != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), svc.path) {
			t.Fatal("private data leaked")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable response")
		}
		return w
	}
	path := "/api/v1/settings/repository-activation/reload"
	body := `{"expected_config_revision":"` + strconv.FormatUint(svc.Snapshot().Revision, 10) + `"}`
	before, _ := os.ReadFile(svc.path)
	request("POST", path, "", body, "", 401)
	request("POST", path, memberToken, body, "", 403)
	request("POST", path, adminToken, body, "https://attacker.example", 403)
	for _, invalid := range []string{`{}`, `null`, `[]`, `{"expected_config_revision":null}`, `{"expected_config_revision":2}`, `{"Expected_Config_Revision":"2"}`, `{"expected_config_revision":"2","expected_config_revision":"2"}`, `{"expected_config_revision":"2","expected_config_revisio\u006e":"2"}`, body + ` {}`, `{"expected_config_revision":"02"}`, `{"expected_config_revision":"+2"}`, `{"expected_config_revision":"0"}`, `{"expected_config_revision":"2.0"}`, `{"expected_config_revision":"18446744073709551616"}`, `{"expected_config_revision":"2","generation":"1"}`, `{"expected_config_revision":"` + strings.Repeat("9", 1024) + `"}`, "\xff"} {
		w := request("POST", path, adminToken, invalid, "http://example.com", 400)
		if !strings.Contains(w.Body.String(), "invalid_activation_request") {
			t.Fatal(w.Body.String())
		}
	}
	after, _ := os.ReadFile(svc.path)
	if string(before) != string(after) || reloadEventCount(t, s) != 0 {
		t.Fatal("invalid request wrote state")
	}
	w := request("GET", "/api/v1/settings", adminToken, "", "", 200)
	var public map[string]json.RawMessage
	if err = json.Unmarshal(w.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	var status repositoryActivationStatus
	if err = json.Unmarshal(public["repository_activation"], &status); err != nil || status.State != "active" || !status.CanReload {
		t.Fatal(status, err)
	}
	// A read-only DTO cannot be posted back as an authorization input.
	request("PUT", "/api/v1/settings", adminToken, w.Body.String(), "http://example.com", 400)
	delete(public, "repository_activation")
	putBody, _ := json.Marshal(public)
	request("PUT", "/api/v1/settings", adminToken, string(putBody), "http://example.com", 200)
	body = `{"expected_config_revision":"` + strconv.FormatUint(svc.Snapshot().Revision, 10) + `"}`
	request("POST", path, adminToken, body, "http://example.com", 200)
	request("POST", path, adminToken, body, "http://example.com", 409)
	if reloadEventCount(t, s) != 1 {
		t.Fatal("HTTP recovery/replay events wrong")
	}
	body = `{"expected_config_revision":"` + strconv.FormatUint(svc.Snapshot().Revision, 10) + `"}`
	activation := svc.activation.Load()
	activation.options.beforePublish = func() error { return errors.New("PRIVATE SQL/file failure") }
	w = request("POST", path, adminToken, body, "http://example.com", 503)
	if !strings.Contains(w.Body.String(), "credential_activation_pending") {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/api/v1/settings", adminToken, "", "", 200)
	if !strings.Contains(w.Body.String(), `"state":"pending"`) {
		t.Fatal(w.Body.String())
	}
	putBody, _ = json.Marshal(svc.Public())
	request("PUT", "/api/v1/settings", adminToken, string(putBody), "http://example.com", 503)
	activation.options.beforePublish = nil
	request("POST", path, adminToken, body, "http://example.com", 200)
	if reloadEventCount(t, s) != 3 {
		t.Fatal("pending recovery lost commit audit")
	}
	// This API does not make an unbound production instance look enabled.
	svc.activation.Store(nil)
	w = request("GET", "/api/v1/settings", adminToken, "", "", 200)
	if strings.Contains(w.Body.String(), "repository_activation") {
		t.Fatal("unbound status exposed")
	}
	body = `{"expected_config_revision":"` + strconv.FormatUint(svc.Snapshot().Revision, 10) + `"}`
	request("POST", path, adminToken, body, "http://example.com", 503)
}
