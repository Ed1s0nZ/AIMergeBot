package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const githubReadAPIVersion = "2022-11-28"
const githubReadResponseLimit int64 = 8 << 20
const githubReadTotalLimit int64 = 32 << 20
const githubReadRequestLimit = 128

// Only normalized classifications escape. Provider bodies and transport errors
// may contain credentials or URLs and must never be wrapped by this error.
type githubReadError struct {
	kind   string
	status int
}

func (e *githubReadError) Error() string {
	if e.status != 0 {
		return fmt.Sprintf("github %s (HTTP %d)", e.kind, e.status)
	}
	return "github " + e.kind
}
func githubReadFailure(kind string) error { return &githubReadError{kind: kind} }

type githubReadClient struct {
	binding   RepositoryBinding
	origin    *url.URL
	token     string
	http      *http.Client
	mu        sync.Mutex
	requests  int
	bytesLeft int64
}

// This constructor does not grant project access. The repository factory must
// authorize frozen target/source identities before it invokes this client.
func newGitHubReadClient(binding RepositoryBinding, credentials IntegrationCredentials) (*githubReadClient, error) {
	valid, err := validateRepositoryBinding(binding)
	if err != nil || binding.Provider != "github" || binding.Revision <= 0 || valid.APIOrigin != binding.APIOrigin || validateIntegrationCredentials("github", credentials, true) != nil {
		return nil, githubReadFailure("invalid_configuration")
	}
	origin, err := canonicalRepositoryAPIOrigin("github", credentials.Endpoint)
	if err != nil || origin != binding.APIOrigin || credentials.Token == "" {
		return nil, githubReadFailure("invalid_configuration")
	}
	for _, ch := range credentials.Token {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return nil, githubReadFailure("invalid_configuration")
		}
	}
	root, err := url.Parse(origin)
	if err != nil {
		return nil, githubReadFailure("invalid_configuration")
	}
	// Callers cannot mutate the network allowlist after construction.
	allow := append([]string(nil), credentials.AllowedNetworks...)
	client := notificationHTTPClient(allow)
	client.Transport.(*http.Transport).MaxResponseHeaderBytes = 64 << 10
	return &githubReadClient{binding: binding, origin: root, token: credentials.Token, http: client, bytesLeft: githubReadTotalLimit}, nil
}

func (g *githubReadClient) requestURL(suffix string, query url.Values) (string, error) {
	if len(suffix) > 1024 || suffix != "" && (!strings.HasPrefix(suffix, "/") || path.Clean(suffix) != suffix || strings.Contains(suffix, "//")) || strings.ContainsAny(suffix, "%?#\\") || !utf8.ValidString(suffix) {
		return "", githubReadFailure("invalid_route")
	}
	for _, segment := range strings.Split(suffix, "/") {
		if segment == "." || segment == ".." {
			return "", githubReadFailure("invalid_route")
		}
	}
	for _, ch := range suffix {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return "", githubReadFailure("invalid_route")
		}
	}
	if len(query) > 10 {
		return "", githubReadFailure("invalid_route")
	}
	for key, values := range query {
		if len(key) > 64 || len(values) > 4 {
			return "", githubReadFailure("invalid_route")
		}
		for _, value := range values {
			if len(value) > 512 {
				return "", githubReadFailure("invalid_route")
			}
		}
	}
	target := *g.origin
	target.Path += "/repos/" + g.binding.FullName + suffix
	target.RawQuery = query.Encode()
	return target.String(), nil
}

// Reserve the complete read allowance so concurrent responses cannot overspend.
// Failed attempts count as requests; only unread byte reservations are refunded.
func (g *githubReadClient) reserve(limit int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit < 1 || limit > githubReadResponseLimit || g.requests >= githubReadRequestLimit || g.bytesLeft < limit+1 {
		return githubReadFailure("read_budget_exceeded")
	}
	g.requests++
	g.bytesLeft -= limit + 1
	return nil
}
func (g *githubReadClient) refund(reserved, used int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.bytesLeft += reserved - used
}

func (g *githubReadClient) get(ctx context.Context, suffix string, query url.Values, limit int64, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := g.requestURL(suffix, query)
	if err != nil {
		return err
	}
	if err = g.reserve(limit); err != nil {
		return err
	}
	var used int64
	defer func() { g.refund(limit+1, used) }()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return githubReadFailure("invalid_route")
	}
	request.Header.Set("Authorization", "Bearer "+g.token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubReadAPIVersion)
	request.Header.Set("User-Agent", "AIMergeBot")
	response, err := g.http.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return githubReadNetworkFailure(err, "request_failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubReadStatusError(response)
	}
	if response.ContentLength > limit {
		return githubReadFailure("read_budget_exceeded")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	used = int64(len(raw))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return githubReadNetworkFailure(err, "response_unavailable")
	}
	if used > limit {
		return githubReadFailure("read_budget_exceeded")
	}
	raw = bytes.TrimSpace(raw)
	if !utf8.Valid(raw) || len(raw) < 2 || raw[0] != '{' || !json.Valid(raw) || json.Unmarshal(raw, out) != nil {
		return githubReadFailure("invalid_response")
	}
	return nil
}

func githubReadNetworkFailure(err error, fallback string) error {
	var temporary net.Error
	if errors.As(err, &temporary) && (temporary.Timeout() || temporary.Temporary()) {
		return &upstreamError{info: RetryInfo{Kind: "temporary_network", Source: "github"}}
	}
	return githubReadFailure(fallback)
}

func githubReadStatusError(response *http.Response) error {
	status := response.StatusCode
	limited := status == http.StatusTooManyRequests || status == http.StatusForbidden && (response.Header.Get("Retry-After") != "" || response.Header.Get("X-RateLimit-Remaining") == "0")
	if limited || status >= 500 && status <= 599 {
		now := time.Now()
		until, state := parseRetryAfter(response.Header.Get("Retry-After"), now)
		if limited {
			until, state = githubRateLimitWait(response.Header, now)
		}
		kind := "upstream_server"
		if limited {
			kind = "rate_limit"
		}
		return &upstreamError{info: RetryInfo{Kind: kind, Source: "github", HTTPStatus: status, HeaderState: state, RetryAfterUntil: until}}
	}
	kind := "request_rejected"
	switch status {
	case 401:
		kind = "credentials_unavailable"
	case 403:
		kind = "permission_unavailable"
	case 404:
		kind = "source_unavailable"
	case 410:
		kind = "api_version_unavailable"
	}
	return &githubReadError{kind: kind, status: status}
}

func githubRateLimitWait(headers http.Header, now time.Time) (string, string) {
	if raw := headers.Get("Retry-After"); raw != "" {
		return parseRetryAfter(raw, now)
	}
	state := "absent"
	if headers.Get("X-RateLimit-Remaining") == "0" {
		raw := headers.Get("X-RateLimit-Reset")
		if raw != "" {
			state = "invalid"
			seconds, err := strconv.ParseUint(raw, 10, 63)
			if err == nil {
				if seconds > uint64(now.Add(maxUpstreamRetryWait).Unix()) {
					return "", "exceeds_limit"
				}
				until := time.Unix(int64(seconds), 0)
				if until.After(now) {
					return until.UTC().Format(time.RFC3339Nano), "valid"
				}
			}
		}
	}
	return now.Add(time.Minute).UTC().Format(time.RFC3339Nano), state
}

func (g *githubReadClient) verifyRepository(ctx context.Context) error {
	var repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err := g.get(ctx, "", nil, 64<<10, &repository); err != nil {
		return err
	}
	if repository.ID != g.binding.RemoteID || !strings.EqualFold(repository.FullName, g.binding.FullName) {
		return githubReadFailure("repository_identity_changed")
	}
	return nil
}
