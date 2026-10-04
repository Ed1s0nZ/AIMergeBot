package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryAfterSyntaxAndSafeWaitBoundary(t *testing.T) {
	received := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value, state string
		offset       time.Duration
	}{
		{"", "absent", 0}, {" 120 ", "valid", 120 * time.Second}, {"0", "valid", 0},
		{"86400", "valid", 24 * time.Hour}, {"86401", "exceeds_limit", 0},
		{"999999999999999999999999999", "exceeds_limit", 0},
		{"-1", "invalid", 0}, {"+1", "invalid", 0}, {"1.2", "invalid", 0}, {"1,2", "invalid", 0},
		{received.Add(time.Minute).Format(http.TimeFormat), "valid", time.Minute},
		{received.Add(-time.Minute).Format(http.TimeFormat), "valid", -time.Minute},
		{received.Add(25 * time.Hour).Format(http.TimeFormat), "exceeds_limit", 0},
	} {
		t.Run(tc.value, func(t *testing.T) {
			until, state := parseRetryAfter(tc.value, received)
			if state != tc.state {
				t.Fatal(state, tc.state)
			}
			if state == "valid" {
				parsed, err := time.Parse(time.RFC3339Nano, until)
				if err != nil || !parsed.Equal(received.Add(tc.offset)) {
					t.Fatal(until, err)
				}
			} else if until != "" {
				t.Fatal("invalid/oversized header retained", until)
			}
		})
	}
}

func TestGitLabRetryAdviceSurvivesSDKWithoutHiddenRequests(t *testing.T) {
	for _, status := range []int{429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"secret-provider-body"}`)
			}))
			defer server.Close()
			repo, err := NewGitLabRepository("synthetic-token", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = repo.Snapshot(context.Background(), 1, 1)
			info, ok := retryFailure(fmt.Errorf("wrapped: %w", err))
			if !ok || info.Source != "gitlab" || info.HTTPStatus != status || info.HeaderState != "valid" || calls.Load() != 1 {
				t.Fatal(info, err, calls.Load())
			}
			if strings.Contains(err.Error(), "secret-provider-body") || strings.Contains(err.Error(), "synthetic-token") {
				t.Fatal("raw upstream data exposed")
			}
		})
	}
}

func TestEinoRetryAdviceSurvivesActualModelSDK(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", time.Now().UTC().Add(time.Minute).Format(http.TimeFormat))
		w.WriteHeader(429)
		fmt.Fprint(w, `{"error":{"message":"secret-provider-body","type":"rate_limit"}}`)
	}))
	defer server.Close()
	auditor := &EinoAuditor{Repository: runRepo{}, Config: AgentConfig{APIKey: "synthetic-token", BaseURL: server.URL + "/v1", Model: "synthetic-retry"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := auditor.Audit(ctx, Snapshot{ProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
	info, ok := retryFailure(err)
	if !ok || info.Source != "model" || info.HTTPStatus != 429 || info.HeaderState != "valid" || calls.Load() != 1 {
		t.Fatal(info, err, calls.Load())
	}
	until, e := time.Parse(time.RFC3339Nano, info.RetryAfterUntil)
	if e != nil || time.Until(until) < 50*time.Second {
		t.Fatal(info, e)
	}
}

type retryNetworkError struct{}

func (retryNetworkError) Error() string   { return "synthetic network error" }
func (retryNetworkError) Timeout() bool   { return true }
func (retryNetworkError) Temporary() bool { return true }

type retryTransportFunc func(*http.Request) (*http.Response, error)

func (f retryTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type retryResponseBody struct{ closed, read bool }

func (b *retryResponseBody) Read([]byte) (int, error) { b.read = true; return 0, io.EOF }
func (b *retryResponseBody) Close() error             { b.closed = true; return nil }

func TestRetryTransportClosesErrorsAndPreservesOtherResponses(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://fixture.invalid/", nil)
	for _, source := range []string{"model", "gitlab"} {
		for _, status := range []int{200, 401, 403, 404, 422, 429, 503} {
			body := &retryResponseBody{}
			response := &http.Response{StatusCode: status, Header: http.Header{}, Body: body}
			transport := upstreamTransport{source: source, base: retryTransportFunc(func(*http.Request) (*http.Response, error) { return response, nil })}
			got, err := transport.RoundTrip(req)
			if status == 429 || status == 503 {
				if got != nil || !retryableError(err) || !body.closed || body.read {
					t.Fatal("retry error body leaked or drained", source, status, err)
				}
			} else if source == "model" && status >= 400 && status < 500 {
				info := UpstreamFailureInfo(err)
				if got != nil || err == nil || retryableError(err) || !body.closed || body.read || info == nil || info.HTTPStatus != status {
					t.Fatal("permanent model body leaked or retry enabled", status, err)
				}
			} else if got != response || err != nil || body.closed || body.read {
				t.Fatal("SDK response changed", source, status, err)
			}
		}
	}
}

func TestRetryTransportNetworkAndCancellation(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://fixture.invalid/", nil)
	for _, cause := range []error{retryNetworkError{}, context.Canceled, context.DeadlineExceeded} {
		transport := upstreamTransport{source: "gitlab", base: retryTransportFunc(func(*http.Request) (*http.Response, error) { return nil, cause })}
		_, err := transport.RoundTrip(req)
		info, ok := retryFailure(err)
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			if ok || !errors.Is(err, cause) {
				t.Fatal("cancellation was retried", err)
			}
		} else if !ok || info.Kind != "temporary_network" || info.Source != "gitlab" {
			t.Fatal(info, err)
		}
	}
}

func TestRetryInfoIsDurableAndHonorsProviderDelay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retry.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().UTC().Add(time.Minute)
	cause := &RetryInfo{Kind: "rate_limit", Source: "model", HTTPStatus: 429, HeaderState: "valid", RetryAfterUntil: deadline.Format(time.RFC3339Nano)}
	child, err := s.failAndRetry(ctx, id, "", "normalized", AuditResult{Summary: "checkpoint"}, nil, 5*time.Second, false, cause)
	if err != nil || child == 0 {
		t.Fatal(child, err)
	}
	parent, err := s.Run(ctx, id)
	if err != nil || parent.RetryInfo == nil || parent.RetryInfo.State != "scheduled" || parent.RetryInfo.DelaySeconds < 59 || parent.Result.Summary != "checkpoint" {
		t.Fatal(parent, err)
	}
	retry, err := s.Run(ctx, child)
	if err != nil || retry.RetryInfo == nil || *retry.RetryInfo != *parent.RetryInfo {
		t.Fatal(retry, err)
	}
	eligible, err := time.Parse(time.RFC3339Nano, retry.RetryAt)
	if err != nil || eligible.Before(deadline) {
		t.Fatal("retry scheduled before provider advice", retry.RetryAt, err)
	}
	if _, err = s.Claim(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("delayed child claimed", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	retry, err = s.RunSummary(ctx, child)
	if err != nil || retry.RetryInfo == nil || retry.RetryInfo.Kind != "rate_limit" {
		t.Fatal("reopen/list lost cause", retry, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET retry_at='' WHERE id=?`, child); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	retry, err = s.Run(ctx, child)
	if err != nil || retry.RetryInfo.Kind != "worker_interrupted" || retry.RetryChildID == 0 {
		t.Fatal("recovery inherited wrong cause", retry, err)
	}
}

func TestRetryAdviceFallbackAndExplicitStopReasons(t *testing.T) {
	for _, tc := range []struct {
		header, state string
		attempt       int
	}{
		{"absent", "scheduled", 0}, {"invalid", "scheduled", 1}, {"exceeds_limit", "wait_exceeds_limit", 0}, {"valid", "exhausted", 2},
	} {
		t.Run(tc.header+tc.state, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec(`UPDATE platform_runs SET retry_attempt=? WHERE id=?`, tc.attempt, id); err != nil {
				t.Fatal(err)
			}
			cause := &RetryInfo{Kind: "rate_limit", Source: "model", HTTPStatus: 429, HeaderState: tc.header, RetryAfterUntil: time.Now().Add(-time.Minute).Format(time.RFC3339Nano)}
			child, err := s.failAndRetry(ctx, id, "", "normalized", AuditResult{}, nil, retryDelay(tc.attempt), false, cause)
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.Run(ctx, id)
			if err != nil || run.RetryInfo == nil || run.RetryInfo.State != tc.state {
				t.Fatal(run, err)
			}
			if tc.state == "scheduled" {
				if child == 0 || run.RetryInfo.DelaySeconds != int64(retryDelay(tc.attempt)/time.Second) {
					t.Fatal("fallback mismatch", child, run.RetryInfo)
				}
			} else if child != 0 || run.RetryInfo.EligibleAt != "" {
				t.Fatal("stopped retry still scheduled", child)
			}
			raw, _ := json.Marshal(run.RetryInfo)
			if strings.Contains(string(raw), "normalized") {
				t.Fatal("metadata contains raw error")
			}
		})
	}
}
