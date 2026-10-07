package platform

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNotificationProtocolsAndBusinessFailures(t *testing.T) {
	summary := NotificationSummary{EventID: "event-1", ProjectID: 1, RunID: 2, Text: "审计摘要", URL: "https://audit.example/#/runs/2"}
	for _, kind := range []string{"feishu", "dingtalk", "wecom", "slack", "teams", "webhook"} {
		req, err := buildNotificationRequest(kind, summary, "test-secret", time.Unix(1700000000, 0))
		if err != nil || !json.Valid(req.Body) {
			t.Fatal(kind, err)
		}
		if strings.Contains(string(req.Body), "test-secret") {
			t.Fatal("secret leaked in payload")
		}
		var body map[string]any
		json.Unmarshal(req.Body, &body)
		switch kind {
		case "feishu":
			if body["msg_type"] != "text" || body["sign"] == nil || body["timestamp"] == nil {
				t.Fatal(body)
			}
		case "dingtalk":
			if body["msgtype"] != "text" || req.Query.Get("sign") == "" {
				t.Fatal(body)
			}
		case "wecom":
			if body["msgtype"] != "text" {
				t.Fatal(body)
			}
		case "slack", "teams":
			if body["text"] == nil {
				t.Fatal(body)
			}
		case "webhook":
			if body["version"] != "aimangebot.notification.v1" || req.Headers["X-AIMergeBot-Signature"] == "" {
				t.Fatal(body)
			}
		}
	}
	for _, tc := range []struct {
		kind        string
		status      int
		body, state string
	}{{"feishu", 200, `{"code":0}`, "delivered"}, {"feishu", 200, `{"code":19021}`, "failed"}, {"feishu", 200, `{}`, "unknown"}, {"dingtalk", 200, `{"errcode":0}`, "delivered"}, {"wecom", 200, `{"errcode":93000}`, "failed"}, {"slack", 200, "ok", "delivered"}, {"slack", 200, "unexpected", "unknown"}, {"teams", 202, "", "accepted"}, {"webhook", 204, "", "accepted"}, {"slack", 429, "private", "retry"}, {"slack", 503, "private", "unknown"}} {
		state, code := notificationResponse(tc.kind, tc.status, []byte(tc.body))
		if state != tc.state || strings.Contains(code, "private") {
			t.Fatal(tc, state, code)
		}
	}
	summary.URL = "javascript:alert(1)"
	if _, err := buildNotificationRequest("slack", summary, "", time.Now()); err == nil {
		t.Fatal("unsafe report URL accepted")
	}
}
