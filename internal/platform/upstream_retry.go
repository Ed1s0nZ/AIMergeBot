package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxUpstreamRetryWait = 24 * time.Hour

// RetryInfo contains normalized public metadata, never raw provider headers or bodies.
type RetryInfo struct {
	Kind            string `json:"kind"`
	Source          string `json:"source"`
	HTTPStatus      int    `json:"http_status,omitempty"`
	HeaderState     string `json:"header_state,omitempty"`
	RetryAfterUntil string `json:"retry_after_until,omitempty"`
	State           string `json:"state"`
	DelaySeconds    int64  `json:"delay_seconds,omitempty"`
	EligibleAt      string `json:"eligible_at,omitempty"`
}

type upstreamError struct {
	info RetryInfo
	err  error
}

func (e *upstreamError) Error() string {
	if e.info.HTTPStatus != 0 {
		return fmt.Sprintf("%s returned HTTP %d", e.info.Source, e.info.HTTPStatus)
	}
	return e.info.Source + " temporary network failure"
}
func (e *upstreamError) Unwrap() error { return e.err }

// parseRetryAfter never clamps a long server delay to an earlier retry time.
func parseRetryAfter(value string, received time.Time) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "absent"
	}
	digits := true
	for _, c := range value {
		if c < '0' || c > '9' {
			digits = false
			break
		}
	}
	if digits {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64(maxUpstreamRetryWait/time.Second) {
			return "", "exceeds_limit"
		}
		return received.UTC().Add(time.Duration(seconds) * time.Second).Format(time.RFC3339Nano), "valid"
	}
	until, err := http.ParseTime(value)
	if err != nil {
		return "", "invalid"
	}
	if until.Sub(received) > maxUpstreamRetryWait {
		return "", "exceeds_limit"
	}
	return until.UTC().Format(time.RFC3339Nano), "valid"
}

type upstreamTransport struct {
	source string
	base   http.RoundTripper
}

func upstreamHTTPClient(source string) *http.Client {
	return &http.Client{Transport: upstreamTransport{source: source, base: http.DefaultTransport}}
}

func (t upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err != nil {
		var temporary net.Error
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && errors.As(err, &temporary) && (temporary.Timeout() || temporary.Temporary()) {
			return nil, &upstreamError{info: RetryInfo{Kind: "temporary_network", Source: t.source}, err: err}
		}
		return response, err
	}
	status := response.StatusCode
	permanentModel := t.source == "model" && status >= 400 && status <= 499 && status != http.StatusTooManyRequests
	if !permanentModel && status != http.StatusTooManyRequests && (status < 500 || status > 599) {
		return response, nil
	}
	until, headerState := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
	// Close immediately rather than buffering an unbounded error body or blocking on a drain.
	response.Body.Close()
	kind := "upstream_server"
	if permanentModel {
		kind = "upstream_client"
		until = ""
		headerState = "absent"
	}
	if status == http.StatusTooManyRequests {
		kind = "rate_limit"
	}
	return nil, &upstreamError{info: RetryInfo{Kind: kind, Source: t.source, HTTPStatus: status, HeaderState: headerState, RetryAfterUntil: until}}
}

// UpstreamFailureInfo exposes normalized metadata only; no response text,
// headers, endpoint URLs or credentials can escape through this API.
func UpstreamFailureInfo(err error) *RetryInfo {
	info, _ := retryFailure(err)
	return info
}
