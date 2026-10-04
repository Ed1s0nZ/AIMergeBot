package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Probe prints status and whitelisted error code only, never provider body,
// URL, authentication header or key. Redirects cannot forward credentials.
func probeModel(ctx context.Context, endpoint, key, model string) error {
	raw, _ := json.Marshal(map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "Respond OK."}}, "max_tokens": 1})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return printProbe("invalid_endpoint", 0, "")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		kind, status := classifyError(err)
		return printProbe(kind, status, "")
	}
	defer response.Body.Close()
	var payload struct {
		Error struct {
			Code any `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&payload)
	code, _ := payload.Error.Code.(string)
	switch code {
	case "invalid_api_key", "model_not_found", "invalid_request_error", "insufficient_quota", "unsupported_parameter", "context_length_exceeded":
	default:
		code = ""
	}
	return printProbe("http_response", response.StatusCode, code)
}

func printProbe(kind string, status int, code string) error {
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"probe_class": kind, "http_status": status, "provider_code": code})
}
