package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NotificationSummary carries no source snippets, tool output, or private trace.
type NotificationSummary struct {
	sourceEventID int64  // Server-side source identity; never sent in payloads.
	Version       string `json:"version"`
	EventID       string `json:"event_id"`
	ProjectID     int    `json:"project_id"`
	RunID         int64  `json:"run_id"`
	Text          string `json:"text"`
	URL           string `json:"url"`
}
type NotificationRequest struct {
	Body    []byte
	Query   url.Values
	Headers map[string]string
}

var ErrNotificationProtocol = errors.New("invalid notification protocol")

func buildNotificationRequest(kind string, summary NotificationSummary, secret string, now time.Time) (NotificationRequest, error) {
	req := NotificationRequest{Query: url.Values{}, Headers: map[string]string{"Content-Type": "application/json"}}
	if len(summary.Text) > 6000 || len(summary.URL) > 2048 || summary.EventID == "" || len(summary.EventID) > 160 {
		return req, ErrNotificationProtocol
	}
	text := summary.Text
	if summary.URL != "" {
		u, err := url.Parse(summary.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return req, ErrNotificationProtocol
		}
		text += "\n" + summary.URL
	}
	var body any
	switch kind {
	case "feishu":
		payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
		if secret != "" {
			timestamp := strconv.FormatInt(now.Unix(), 10)
			mac := hmac.New(sha256.New, []byte(timestamp+"\n"+secret))
			payload["timestamp"] = timestamp
			payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
		body = payload
	case "dingtalk":
		body = map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
		if secret != "" {
			timestamp := strconv.FormatInt(now.UnixMilli(), 10)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(timestamp + "\n" + secret))
			req.Query.Set("timestamp", timestamp)
			req.Query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		}
	case "wecom":
		body = map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
	case "slack", "teams":
		body = map[string]string{"text": text}
	case "webhook":
		summary.Version = "aimangebot.notification.v1"
		body = summary
	default:
		return req, ErrNotificationProtocol
	}
	data, err := json.Marshal(body)
	if err != nil {
		return req, ErrNotificationProtocol
	}
	req.Body = data
	if kind == "webhook" && secret != "" {
		timestamp := strconv.FormatInt(now.Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "."))
		mac.Write(data)
		req.Headers["X-AIMergeBot-Timestamp"] = timestamp
		req.Headers["X-AIMergeBot-Signature"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Headers["X-AIMergeBot-Event-ID"] = summary.EventID
	}
	return req, nil
}

// A webhook acceptance is not proof of downstream Teams/workflow delivery.
// Do not expose provider response bodies (which may contain credentials).
func notificationResponse(kind string, status int, body []byte) (string, string) {
	if len(body) > 65536 {
		return "unknown", "response_too_large"
	}
	if status == 429 {
		return "retry", "rate_limited"
	}
	if status >= 500 {
		return "unknown", "provider_server_error"
	}
	if status < 200 || status >= 300 {
		return "failed", "provider_http_rejected"
	}
	switch kind {
	case "feishu":
		var result struct {
			Code       *int `json:"code"`
			StatusCode *int `json:"StatusCode"`
		}
		if json.Unmarshal(body, &result) != nil {
			return "unknown", "invalid_response"
		}
		code := result.Code
		if code == nil {
			code = result.StatusCode
		}
		if code == nil {
			return "unknown", "missing_business_status"
		}
		if *code != 0 {
			return "failed", "provider_business_rejected"
		}
		return "delivered", ""
	case "dingtalk", "wecom":
		var result struct {
			Code *int `json:"errcode"`
		}
		if json.Unmarshal(body, &result) != nil || result.Code == nil {
			return "unknown", "invalid_response"
		}
		if *result.Code != 0 {
			return "failed", "provider_business_rejected"
		}
		return "delivered", ""
	case "slack":
		if strings.TrimSpace(string(body)) != "ok" {
			return "unknown", "unexpected_response"
		}
		return "delivered", ""
	case "teams", "webhook":
		return "accepted", ""
	default:
		return "failed", "unsupported_channel"
	}
}
