package platform

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func githubClientFixture(t *testing.T, handler http.HandlerFunc) (*githubReadClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	binding := RepositoryBinding{Revision: 1, Provider: "github", APIOrigin: server.URL, RemoteID: 7, FullName: "Org/Repo", IntegrationID: 1}
	client, err := newGitHubReadClient(binding, IntegrationCredentials{Endpoint: server.URL, Token: "fixture-only-token"})
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = server.Client().Transport // Test-only TLS trust and loopback access.
	return client, server
}

func TestGitHubReadClientHeadersIdentityAndEnterpriseRoot(t *testing.T) {
	client, server := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer fixture-only-token" || r.Header.Get("X-GitHub-Api-Version") != githubReadAPIVersion || r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("unexpected read request headers")
		}
		if r.URL.Path != "/api/v3/repos/Org/Repo" || r.URL.RawQuery != "" {
			t.Error("unexpected repository route")
		}
		fmt.Fprint(w, `{"id":7,"full_name":"org/repo","future_field":true}`)
	})
	binding := client.binding
	binding.APIOrigin = server.URL + "/api/v3"
	enterprise, err := newGitHubReadClient(binding, IntegrationCredentials{Endpoint: binding.APIOrigin, Token: "fixture-only-token"})
	if err != nil {
		t.Fatal(err)
	}
	enterprise.http.Transport = server.Client().Transport
	if err = enterprise.verifyRepository(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(enterprise)
	if err != nil || string(raw) != "{}" {
		t.Fatal("read client must not expose credentials through JSON")
	}
}

func TestGitHubReadClientInvalidConfigurationAndRoutesMakeNoRequests(t *testing.T) {
	var hits atomic.Int32
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, `{}`) })
	for _, suffix := range []string{"https://evil.example", "//evil.example/x", "/../x", "/git/../x", "/git//x", "/git/%2e%2e/x", "/git/x?token=secret", "/git/x#fragment", "/git/\\x", "/git/\n", "/git/ x", "/" + strings.Repeat("a", 1024)} {
		var out map[string]any
		if err := client.get(context.Background(), suffix, nil, 100, &out); err == nil {
			t.Errorf("invalid route accepted: %q", suffix)
		}
	}
	for _, mutate := range []func(*RepositoryBinding, *IntegrationCredentials){
		func(b *RepositoryBinding, c *IntegrationCredentials) { b.Provider = "gitlab" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { b.Revision = 0 },
		func(b *RepositoryBinding, c *IntegrationCredentials) { b.FullName = "org/../repo" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { b.APIOrigin = "http://example.com" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { c.Endpoint = "https://other.example" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { c.Token = "" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { c.Token = "foo\r\nbar" },
		func(b *RepositoryBinding, c *IntegrationCredentials) { c.Token = strings.Repeat("x", 4097) },
		func(b *RepositoryBinding, c *IntegrationCredentials) { c.AllowedNetworks = []string{"0.0.0.0/0"} },
	} {
		b := client.binding
		c := IntegrationCredentials{Endpoint: b.APIOrigin, Token: "fixture-only-token"}
		mutate(&b, &c)
		if _, err := newGitHubReadClient(b, c); err == nil {
			t.Error("invalid configuration accepted")
		}
	}
	var out map[string]any
	if client.get(context.Background(), "", url.Values{"page": {strings.Repeat("x", 513)}}, 100, &out) == nil {
		t.Fatal("oversized query accepted")
	}
	if hits.Load() != 0 {
		t.Fatal("invalid routes/configuration issued HTTP requests")
	}
	if client.requests != 0 {
		t.Fatal("invalid routes consumed upstream request budget")
	}
}

func TestGitHubReadClientNeverFollowsRedirectOrExposesErrors(t *testing.T) {
	var destinationHits atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationHits.Add(1) }))
	defer destination.Close()
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/fixture-only-token", http.StatusFound)
	})
	var out map[string]any
	err := client.get(context.Background(), "", nil, 100, &out)
	if err == nil || strings.Contains(err.Error(), "fixture-only-token") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatal("redirect error must be sanitized")
	}
	if destinationHits.Load() != 0 {
		t.Fatal("redirect received a credential-bearing request")
	}
}

func TestGitHubReadClientStatusAndRateLimitClassification(t *testing.T) {
	for _, status := range []int{401, 403, 404, 410, 422, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"fixture-only-token at secret-host"}`)
			})
			var out map[string]any
			err := client.get(context.Background(), "", nil, 100, &out)
			if err == nil || strings.Contains(err.Error(), "fixture-only-token") || strings.Contains(err.Error(), "secret-host") {
				t.Fatal("provider error not sanitized")
			}
			info := UpstreamFailureInfo(err)
			if status == 403 || status == 429 || status >= 500 {
				if info == nil || info.Source != "github" || info.HTTPStatus != status || info.HeaderState != "valid" {
					t.Fatalf("incorrect retry classification: %+v", info)
				}
			} else if info != nil {
				t.Fatal("permanent error became retryable")
			}
		})
	}
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	var out map[string]any
	if info := UpstreamFailureInfo(client.get(context.Background(), "", nil, 100, &out)); info != nil {
		t.Fatal("ordinary permission denial marked as rate limit")
	}
	client, _ = githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(403)
	})
	if info := UpstreamFailureInfo(client.get(context.Background(), "", nil, 100, &out)); info == nil || info.Kind != "rate_limit" {
		t.Fatal("primary rate limit not recognized")
	}
}

func TestGitHubReadClientBoundedSingleJSONObject(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{"ok":true} {}`, `{"ok":true} garbage`, "{\"x\":\"\xff\"}", strings.Repeat(" ", 101)} {
		t.Run(fmt.Sprintf("body_%d", len(body)), func(t *testing.T) {
			client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			var out map[string]any
			if err := client.get(context.Background(), "", nil, 100, &out); err == nil {
				t.Fatal("invalid/oversized JSON accepted")
			}
		})
	}
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		fmt.Fprint(w, strings.Repeat("x", 101))
	})
	var out map[string]any
	if err := client.get(context.Background(), "", nil, 100, &out); err == nil {
		t.Fatal("chunked oversized body accepted")
	}
	if client.bytesLeft != githubReadTotalLimit-101 {
		t.Fatal("bytes actually read were not accounted")
	}
	client, _ = githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"name":"路径.go"}`) })
	if err := client.get(context.Background(), "", nil, 100, &out); err != nil {
		t.Fatal(err)
	}
	if out["name"] != "路径.go" {
		t.Fatal("Unicode response changed")
	}
}

func TestGitHubReadClientBudgetsAndConcurrentReservations(t *testing.T) {
	var hits atomic.Int32
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, `{}`) })
	client.requests = githubReadRequestLimit - 4
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out map[string]any
			if client.get(context.Background(), "", nil, 100, &out) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 4 || successes.Load() != 4 || client.requests != githubReadRequestLimit {
		t.Fatal("request budget exceeded under concurrency")
	}
	if client.bytesLeft != githubReadTotalLimit-8 {
		t.Fatal("byte refund did not match successful bodies")
	}
	client.requests = 0
	client.bytesLeft = 100
	var out map[string]any
	if client.get(context.Background(), "", nil, 100, &out) == nil || hits.Load() != 4 {
		t.Fatal("insufficient byte reservation issued a request")
	}
	for _, limit := range []int64{0, -1, githubReadResponseLimit + 1} {
		if client.get(context.Background(), "", nil, limit, &out) == nil {
			t.Fatal("invalid response budget accepted")
		}
	}
}

func TestGitHubReadClientIdentityDriftAndCancellation(t *testing.T) {
	for _, body := range []string{`{"id":8,"full_name":"Org/Repo"}`, `{"id":7,"full_name":"Org/Other"}`, `{}`} {
		client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		if client.verifyRepository(context.Background()) == nil {
			t.Fatal("repository identity drift accepted")
		}
	}
	var hits atomic.Int32
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out map[string]any
	if err := client.get(ctx, "", nil, 100, &out); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not preserved")
	}
	if hits.Load() != 0 || client.requests != 0 {
		t.Fatal("canceled request issued HTTP")
	}
}

func TestGitHubReadClientProductionTransportAndEncodedQuery(t *testing.T) {
	client, server := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
	allow := []string{"10.0.0.0/8"}
	production, err := newGitHubReadClient(client.binding, IntegrationCredentials{Endpoint: server.URL, Token: "fixture-only-token", AllowedNetworks: allow})
	if err != nil {
		t.Fatal(err)
	}
	allow[0] = "127.0.0.0/8"
	transport, ok := production.http.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig.MinVersion < tls.VersionTLS12 || production.http.Timeout != 15*time.Second || !transport.DisableCompression {
		t.Fatal("production transport lacks fixed security limits")
	}
	conn, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:443")
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("production transport accepted loopback")
	}
	// Query values are encoded; they cannot override the repository path/origin.
	target, err := production.requestURL("/git/trees/abc", url.Values{"recursive": {"https://other.example/?token=x"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(target)
	if parsed.Host != production.origin.Host || parsed.Path != "/repos/Org/Repo/git/trees/abc" {
		t.Fatal("query changed the fixed request target")
	}
}

func TestGitHubReadClientCancellationDuringRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	client, _ := githubClientFixture(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { var out map[string]any; result <- client.get(ctx, "", nil, 100, &out) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("controlled request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("active request cancellation not preserved")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not cancel")
	}
}

type githubTimeoutFixture struct{}

func (githubTimeoutFixture) Error() string   { return "fixture-only-token at secret-host" }
func (githubTimeoutFixture) Timeout() bool   { return true }
func (githubTimeoutFixture) Temporary() bool { return true }
func TestGitHubReadClientTemporaryNetworkErrorsAreSanitized(t *testing.T) {
	err := githubReadNetworkFailure(&url.Error{Op: "Get", URL: "https://secret-host", Err: githubTimeoutFixture{}}, "request_failed")
	info := UpstreamFailureInfo(err)
	if info == nil || info.Kind != "temporary_network" || info.Source != "github" {
		t.Fatal("temporary network classification missing")
	}
	if strings.Contains(err.Error(), "fixture-only-token") || strings.Contains(err.Error(), "secret-host") || errors.Unwrap(err) != nil {
		t.Fatal("raw network error escaped")
	}
}

func TestGitHubReadClientPrimaryAndSecondaryWaits(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		headers http.Header
		state   string
		seconds int
	}{
		{"retry_after", http.Header{"Retry-After": {"120"}, "X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(time.Hour).Unix())}}, "valid", 120},
		{"primary_reset", http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(time.Hour).Unix())}}, "valid", 3600},
		{"huge_reset", http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(25 * time.Hour).Unix())}}, "exceeds_limit", 0},
		{"bad_reset", http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"fixture-only-token"}}, "invalid", 60},
		{"absent_reset", http.Header{"X-Ratelimit-Remaining": {"0"}}, "absent", 60},
		{"secondary_wait", http.Header{}, "absent", 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, state := githubRateLimitWait(tc.headers, now)
			if state != tc.state {
				t.Fatal("incorrect header state")
			}
			if tc.seconds == 0 {
				if until != "" {
					t.Fatal("oversized wait clamped earlier")
				}
				return
			}
			parsed, err := time.Parse(time.RFC3339Nano, until)
			if err != nil || !parsed.Equal(now.Add(time.Duration(tc.seconds)*time.Second)) {
				t.Fatal("upstream wait shortened or malformed")
			}
		})
	}
}
