package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTrustedProxyValidationAndPersistence(t *testing.T) {
	for _, raw := range []string{"*", "localhost", "", "0.0.0.0", "::", "0.0.0.0/0", "::/0", "::ffff:0.0.0.0", "::ffff:0.0.0.0/96", "not-an-IP", "ff02::1", "fe80::1%en0"} {
		if validateTrustedProxies([]string{raw}) == nil {
			t.Fatalf("unsafe/invalid proxy accepted: %s", raw)
		}
	}
	proxies := []string{"127.0.0.1", "10.0.1.0/24", "::1", "2001:db8:abcd::/48"}
	if err := validateTrustedProxies(proxies); err != nil {
		t.Fatal(err)
	}
	if validateTrustedProxies(make([]string, 33)) == nil {
		t.Fatal("unbounded trust list")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	svc, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.TrustedProxies = proxies
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	proxies[0] = "0.0.0.0/0"
	if svc.Snapshot().TrustedProxies[0] != "127.0.0.1" {
		t.Fatal("caller mutated live trust list")
	}
	copy := svc.Snapshot()
	copy.TrustedProxies[0] = "0.0.0.0/0"
	reopened, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil || !sameTrustedProxies(svc.Snapshot().TrustedProxies, reopened.Snapshot().TrustedProxies) {
		t.Fatal("proxy setting not durable/isolated", err)
	}
	public := svc.Public()
	delete(public, "trusted_proxies") // older API consumers retain the existing trust configuration
	raw, _ := json.Marshal(public)
	decoded, err := svc.DecodePublic(raw)
	if err != nil || !sameTrustedProxies(decoded.TrustedProxies, svc.Snapshot().TrustedProxies) {
		t.Fatal("omitted trust list reset configuration", err)
	}
}

func TestTrustedProxySettingsHTTPRequiresRestartAndRejectsUniversalTrust(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	(&HTTP{Store: s, Settings: svc}).Register(router)
	save := func(proxies []string) *httptest.ResponseRecorder {
		public := svc.Public()
		public["trusted_proxies"] = proxies
		raw, _ := json.Marshal(public)
		req := httptest.NewRequest("PUT", "/api/v1/settings", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := save([]string{"127.0.0.1"})
	var response struct {
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || !response.RestartRequired {
		t.Fatalf("missing restart notification: %s", w.Body.String())
	}
	// Later saves and page reloads must not hide a still-pending restart.
	w = save([]string{"127.0.0.1"})
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || !response.RestartRequired {
		t.Fatal("second save lost pending restart")
	}
	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || !response.RestartRequired {
		t.Fatal("reload lost pending restart", w.Body.String())
	}
	public := svc.Public()
	public["restart_required"] = true
	raw, _ := json.Marshal(public)
	if _, err := svc.DecodePublic(raw); err != nil {
		t.Fatal("read-only restart field broke settings round trip", err)
	}
	if w = save([]string{"0.0.0.0/0"}); w.Code != 400 || !sameTrustedProxies(svc.Snapshot().TrustedProxies, []string{"127.0.0.1"}) {
		t.Fatal("invalid trust updated persisted configuration", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"invalid_trusted_proxies"`) {
		t.Fatal("proxy validation lacks stable client error code")
	}
	if w = save([]string{}); w.Code != 200 {
		t.Fatal("cannot remove proxy trust", w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.RestartRequired {
		t.Fatal("restoring active configuration still requires restart")
	}
}
