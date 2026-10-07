package platform

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationTargetAddressBoundaries(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "::1", "169.254.169.254", "0.0.0.0", "fe80::1", "224.0.0.1", "10.1.2.3", "192.168.1.1"} {
		if allowedNotificationIP(net.ParseIP(value), nil) {
			t.Fatal("unsafe address allowed", value)
		}
	}
	if !allowedNotificationIP(net.ParseIP("10.1.2.3"), []string{"10.0.0.0/8"}) {
		t.Fatal("explicit private subnet rejected")
	}
	if allowedNotificationIP(net.ParseIP("127.0.0.1"), []string{"0.0.0.0/0"}) {
		t.Fatal("loopback allowlisted")
	}
	if err := validateIntegrationCredentials("slack", IntegrationCredentials{Endpoint: "https://hooks.example", AllowedNetworks: []string{"0.0.0.0/0"}}, true); err == nil {
		t.Fatal("broad private allowlist accepted")
	}
}
func TestNotificationHTTPSRequestAndResponseClassification(t *testing.T) {
	var payload map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("request protocol")
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"errcode":93000,"errmsg":"PRIVATE SECRET"}`))
	}))
	defer server.Close()
	state, code := sendHTTPNotification(context.Background(), "wecom", IntegrationCredentials{Endpoint: server.URL}, NotificationSummary{EventID: "test", Text: "summary"}, server.Client())
	if state != "failed" || code != "provider_business_rejected" || payload["msgtype"] != "text" {
		t.Fatal(state, code, payload)
	}
	if strings.Contains(code, "PRIVATE") {
		t.Fatal("provider response leaked")
	}
	state, code = sendHTTPNotification(context.Background(), "slack", IntegrationCredentials{Endpoint: "http://invalid"}, NotificationSummary{EventID: "test", Text: "summary"}, server.Client())
	if state != "failed" || code != "invalid_configuration" {
		t.Fatal(state, code)
	}
}
