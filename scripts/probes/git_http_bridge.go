//go:build ignore

// Feasibility probe only. This is not a production repository transport.
// Run from the repository root: go run scripts/probes/git_http_bridge.go
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func need(ok bool, message string) {
	if !ok {
		fmt.Fprintln(os.Stderr, "probe assertion:", message)
		panic("probe failed")
	}
}
func env(dir string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_NO_REPLACE_OBJECTS=1"}
}
func command(dir string, extra []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defaults := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "http.followRedirects=false", "-c", "protocol.allow=never", "-c", "protocol.http.allow=always", "-c", "protocol.version=2", "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
	cmd := exec.CommandContext(ctx, "git", append(defaults, args...)...)
	cmd.Dir = dir
	cmd.Env = append(env(dir), extra...)
	out, err := cmd.Output()
	return string(out), err // Never print subprocess errors or credentials.
}
func mustGit(dir string, args ...string) string {
	out, err := command(dir, nil, args...)
	need(err == nil, "fixture Git command failed")
	return strings.TrimSpace(out)
}

// Observe the actual advertisement, not merely a requested protocol header.
type advertisementWriter struct {
	http.ResponseWriter
	prefix []byte
	v2     *atomic.Bool
}

func (w *advertisementWriter) Write(raw []byte) (int, error) {
	n := min(len(raw), 128-len(w.prefix))
	w.prefix = append(w.prefix, raw[:n]...)
	if bytes.Contains(w.prefix, []byte("version 2\n")) {
		w.v2.Store(true)
	}
	return w.ResponseWriter.Write(raw)
}

type ledger struct {
	mu                    sync.Mutex
	requests, maxRequests int
	consumed, maxBytes    int64
	exhausted             bool
}

func (l *ledger) reserve() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.exhausted || l.requests >= l.maxRequests {
		return false
	}
	l.requests++
	return true
}
func (l *ledger) remaining() int64 { l.mu.Lock(); defer l.mu.Unlock(); return l.maxBytes - l.consumed }
func (l *ledger) consume(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.consumed += int64(n)
	if l.consumed > l.maxBytes {
		l.exhausted = true
		return false
	}
	return true
}
func (l *ledger) sealRequests() { l.mu.Lock(); defer l.mu.Unlock(); l.maxRequests = l.requests }

type bridge struct {
	client             *http.Client
	remote, capability string
	tokens             map[string]string
	budget             *ledger
	allowed            atomic.Bool
	badLocalAuth       atomic.Bool
}

func (b *bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.capability)) != 1 {
		http.Error(w, "denied", 403)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 2 || b.tokens[parts[0]] == "" || r.URL.RawPath != "" {
		http.Error(w, "route", 404)
		return
	}
	suffix := strings.Join(parts[1:], "/")
	if !(r.Method == "GET" && suffix == "info/refs" && r.URL.RawQuery == "service=git-upload-pack" || r.Method == "POST" && suffix == "git-upload-pack" && r.URL.RawQuery == "" && r.Header.Get("Content-Type") == "application/x-git-upload-pack-request") {
		http.Error(w, "route", 404)
		return
	}
	if !b.allowed.Load() || !b.budget.reserve() {
		http.Error(w, "unavailable", 503)
		return
	}
	for _, token := range b.tokens {
		if strings.Contains(r.Header.Get("Authorization"), token) {
			b.badLocalAuth.Store(true)
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 || !b.budget.consume(len(body)) {
		http.Error(w, "budget", 503)
		return
	}
	target := b.remote + "/" + parts[0] + ".git/" + suffix
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "request", 503)
		return
	}
	request.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("fixture:"+b.tokens[parts[0]])))
	for _, name := range []string{"Content-Type", "Content-Encoding", "Git-Protocol"} {
		if v := r.Header.Get(name); v != "" {
			request.Header.Set(name, v)
		}
	}
	response, err := b.client.Do(request)
	if err != nil {
		http.Error(w, "upstream", 503)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || !b.allowed.Load() || response.Header.Get("Content-Encoding") != "" {
		http.Error(w, "unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	// Deliberately copy no credentials, cookies, redirects or arbitrary headers.
	buffer := make([]byte, 4096)
	for {
		if !b.allowed.Load() {
			return
		}
		remaining := b.budget.remaining()
		if remaining < 0 {
			return
		}
		size := min(int64(len(buffer)), remaining+1)
		n, readErr := response.Body.Read(buffer[:size])
		if !b.budget.consume(n) || !b.allowed.Load() {
			return
		}
		if n > 0 {
			if _, err = w.Write(buffer[:n]); err != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}

func run() map[string]any {
	root, err := os.MkdirTemp("", "aim-git-bridge-probe-*")
	need(err == nil, "temp directory")
	defer os.RemoveAll(root)
	serverRoot := filepath.Join(root, "upstream")
	need(os.Mkdir(serverRoot, 0700) == nil, "fixture root")
	ids := map[string]string{}
	tokens := map[string]string{"a": "SYNTHETIC-UPSTREAM-A", "b": "SYNTHETIC-UPSTREAM-B", "c": "SYNTHETIC-UPSTREAM-C"}
	for _, name := range []string{"a", "b", "c"} {
		objectFormat := "sha1"
		if name == "c" {
			objectFormat = "sha256"
		}
		seed := filepath.Join(root, "seed-"+name)
		need(os.Mkdir(seed, 0700) == nil, "seed")
		mustGit(seed, "init", "--bare", "--template=", "--object-format="+objectFormat, filepath.Join(serverRoot, name+".git"))
		mustGit(seed, "init", "--template=", "--object-format="+objectFormat, seed)
		mustGit(seed, "config", "user.name", "Fixture")
		mustGit(seed, "config", "user.email", "fixture@example.invalid")
		for _, version := range []string{"base", "pinned", "new"} {
			need(os.WriteFile(filepath.Join(seed, "evidence.txt"), []byte("synthetic-object-"+name+"-"+version+"\n"), 0600) == nil, "fixture blob")
			mustGit(seed, "add", "--", "evidence.txt")
			mustGit(seed, "commit", "-m", version)
			if version == "pinned" {
				ids[name] = mustGit(seed, "rev-parse", "HEAD")
			}
		}
		mustGit(seed, "-c", "protocol.file.allow=always", "push", filepath.Join(serverRoot, name+".git"), "HEAD:refs/heads/main")
		mustGit(filepath.Join(serverRoot, name+".git"), "config", "http.receivepack", "false")
		mustGit(filepath.Join(serverRoot, name+".git"), "config", "http.getanyfile", "false")
		mustGit(filepath.Join(serverRoot, name+".git"), "config", "uploadpack.allowReachableSHA1InWant", "true")
	}
	gitDir := mustGit(root, "--exec-path")
	var upstreamCalls, protocol2, wrongCredential, trapCalls, leakedCallerHeader atomic.Int64
	var revoke, redirect, v2Advertisement atomic.Bool
	var active atomic.Pointer[bridge]
	trap := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { trapCalls.Add(1); w.WriteHeader(500) }))
	defer trap.Close()
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Proxy-Authorization") != "" {
			leakedCallerHeader.Add(1)
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		name := strings.TrimSuffix(parts[0], ".git")
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("fixture:"+tokens[name]))
		if tokens[name] == "" || r.Header.Get("Authorization") != expected {
			wrongCredential.Add(1)
			w.WriteHeader(403)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, trap.URL+"/must-not-receive-token", 302)
			return
		}
		if r.Header.Get("Git-Protocol") == "version=2" {
			protocol2.Add(1)
		}
		if revoke.Load() {
			active.Load().allowed.Store(false)
		}
		r.Header.Del("Authorization") // Fixture CGI never needs the synthetic upstream secret.
		w.Header().Set("Set-Cookie", "must-not-forward=synthetic")
		handler := &cgi.Handler{Path: filepath.Join(gitDir, "git-http-backend"), Root: "/", Dir: serverRoot, Env: append(env(serverRoot), "GIT_PROJECT_ROOT="+serverRoot, "GIT_HTTP_EXPORT_ALL=1", "GIT_PROTOCOL="+r.Header.Get("Git-Protocol"))}
		if r.Method == http.MethodGet {
			handler.ServeHTTP(&advertisementWriter{ResponseWriter: w, v2: &v2Advertisement}, r)
		} else {
			handler.ServeHTTP(w, r)
		}
	}))
	defer remote.Close()
	client := remote.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Transport.(*http.Transport).DisableCompression = true
	capability := make([]byte, 24)
	_, err = rand.Read(capability)
	need(err == nil, "local capability")
	makeBridge := func() *bridge {
		b := &bridge{client: client, remote: remote.URL, capability: hex.EncodeToString(capability), tokens: tokens, budget: &ledger{maxRequests: 64, maxBytes: 16 << 20}}
		b.allowed.Store(true)
		active.Store(b)
		return b
	}
	b := makeBridge()
	local := httptest.NewServer(b)
	defer local.Close()
	gitEnv := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http." + local.URL + "/.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Bearer " + b.capability}
	fetch := func(name, version string) (string, error) {
		dir, e := os.MkdirTemp(root, "consumer-*")
		need(e == nil, "consumer directory")
		objectFormat := "sha1"
		if name == "c" {
			objectFormat = "sha256"
		}
		mustGit(dir, "init", "--bare", "--template=", "--object-format="+objectFormat, dir)
		_, e = command(dir, gitEnv, "-c", "protocol.version="+version, "fetch", "--no-tags", "--no-recurse-submodules", "--depth=1", local.URL+"/"+name, ids[name])
		if !b.allowed.Load() {
			e = fmt.Errorf("operation authorization changed")
		}
		if e == nil {
			content, readErr := command(dir, nil, "show", ids[name]+":evidence.txt")
			if readErr != nil {
				return "", readErr
			}
			return strings.TrimSpace(content), nil
		}
		return "", e
	}
	type fetchOutcome struct {
		name, content string
		err           error
	}
	results := make(chan fetchOutcome, 3)
	for _, name := range []string{"a", "b", "c"} {
		go func(name string) { content, err := fetch(name, "2"); results <- fetchOutcome{name, content, err} }(name)
	}
	for range 3 {
		result := <-results
		need(result.err == nil && result.content == "synthetic-object-"+result.name+"-pinned", "concurrent fixed SHA Git fetch through bridge")
	}
	need(wrongCredential.Load() == 0 && !b.badLocalAuth.Load() && protocol2.Load() > 0 && v2Advertisement.Load(), "credential isolation and negotiated v2")
	legacyContent, legacyErr := fetch("a", "0")
	need(legacyErr == nil && legacyContent == "synthetic-object-a-pinned", "smart HTTP v0 compatibility")
	gitFetchRequests := upstreamCalls.Load()
	check, _ := http.NewRequest("GET", local.URL+"/a/info/refs?service=git-upload-pack", nil)
	check.Header.Set("Authorization", "Bearer "+b.capability)
	check.Header.Set("Cookie", "caller=must-not-forward")
	check.Header.Set("Proxy-Authorization", "must-not-forward")
	checked, checkErr := http.DefaultClient.Do(check)
	need(checkErr == nil, "authorized header filtering request")
	_, checkErr = io.Copy(io.Discard, checked.Body)
	checked.Body.Close()
	need(checkErr == nil && checked.StatusCode == 200 && checked.Header.Get("Set-Cookie") == "" && leakedCallerHeader.Load() == 0, "header filtering")
	successes := upstreamCalls.Load()
	need(successes > 1, "upstream observations")
	// All routes share one exhausted request ledger; changing repositories cannot reset it.
	b.budget.sealRequests()
	_, err = fetch("b", "2")
	need(err != nil && upstreamCalls.Load() == successes, "shared request budget")
	// Unauthorized and non-fetch routes make zero upstream requests.
	for _, attempt := range []struct{ method, path string }{
		{"GET", "/a/git-receive-pack"}, {"POST", "/a/git-receive-pack"}, {"GET", "/a/objects/private"}, {"GET", "/unknown/info/refs?service=git-upload-pack"}, {"HEAD", "/a/info/refs?service=git-upload-pack"}, {"GET", "/a/info/refs?service=git-receive-pack"},
	} {
		request, _ := http.NewRequest(attempt.method, local.URL+attempt.path, nil)
		request.Header.Set("Authorization", "Bearer "+b.capability)
		response, e := http.DefaultClient.Do(request)
		need(e == nil, "local forbidden route")
		response.Body.Close()
		need(response.StatusCode == 404, "forbidden route rejected")
	}
	response, e := http.Get(local.URL + "/a/info/refs?service=git-upload-pack")
	need(e == nil, "local capability rejection")
	response.Body.Close()
	need(response.StatusCode == 403 && upstreamCalls.Load() == successes, "capability and routes zero upstream")
	// A new local server/session is required for each independent experiment.
	newExperiment := func() {
		local.Close()
		b = makeBridge()
		local = httptest.NewServer(b)
		gitEnv = []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http." + local.URL + "/.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Bearer " + b.capability}
	}
	defer func() { local.Close() }()
	newExperiment()
	redirect.Store(true)
	before := upstreamCalls.Load()
	_, err = fetch("a", "2")
	need(err != nil && upstreamCalls.Load() == before+1 && trapCalls.Load() == 0, "redirect must not reach destination")
	redirect.Store(false)
	newExperiment()
	revoke.Store(true)
	b.allowed.Store(true)
	before = upstreamCalls.Load()
	_, err = fetch("a", "2")
	need(err != nil && upstreamCalls.Load() == before+1, "revoke during discovery stops next request")
	revoke.Store(false)
	newExperiment()
	b.budget.mu.Lock()
	b.budget.maxBytes = 16
	b.budget.mu.Unlock()
	before = upstreamCalls.Load()
	_, err = fetch("a", "2")
	b.budget.mu.Lock()
	exhausted := b.budget.exhausted
	b.budget.mu.Unlock()
	need(err != nil && upstreamCalls.Load() == before+1 && exhausted, "body budget stops fetch")
	// No upstream token should be persisted in any child workspace/config/FETCH_HEAD.
	for _, token := range tokens {
		need(!strings.Contains(strings.Join(gitEnv, "\n"), token), "upstream secret in Git environment")
	}
	need(filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if info.Size() > 1<<20 {
			return nil
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		for _, token := range tokens {
			if bytes.Contains(raw, []byte(token)) {
				return fmt.Errorf("secret persisted")
			}
		}
		return nil
	}) == nil, "upstream secret persisted in workspace")
	return map[string]any{"synthetic_only": true, "git_version": mustGit(root, "--version"), "three_repository_fetches": true, "sha256_fetch": true, "non_tip_sha_fetch": true, "concurrent_credentials": true, "successful_git_fetch_requests": gitFetchRequests, "header_filtering": true, "upstream_requests_before_faults": successes, "v2_observed": protocol2.Load() > 0 && v2Advertisement.Load(), "smart_http_v0_fetch": true, "wrong_credentials": wrongCredential.Load(), "shared_request_budget": true, "invalid_routes_and_capability_zero_upstream": true, "redirect_target_requests": trapCalls.Load(), "revoke_stops_next_request": true, "body_limit_stops_fetch": true, "upstream_secrets_absent_from_git_env_and_workspace": true, "production_transport_tested": false}
}
func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "Git bridge feasibility probe failed; no provider/credential output recorded")
			os.Exit(1)
		}
	}()
	need(json.NewEncoder(os.Stdout).Encode(run()) == nil, "encode proof")
}
