// Isolated pinned-library comparison, not a production repository adapter.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/client"
	format "github.com/go-git/go-git/v6/plumbing/format/config"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
)

var errPolicy = errors.New("probe policy unavailable")

func need(ok bool, message string) {
	if !ok {
		fmt.Fprintln(os.Stderr, "probe assertion:", message)
		panic("probe failed")
	}
}
func gitEnv(dir string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "LANG=C.UTF-8"}
}
func command(dir string, args ...string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	a := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "protocol.allow=never", "-c", "protocol.file.allow=always", "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
	cmd := exec.CommandContext(ctx, "git", append(a, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv(dir)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
func mustGit(dir string, args ...string) string {
	out, err := command(dir, args...)
	need(err == nil, "fixture/local Git command")
	return out
}

type limits struct {
	mu                    sync.Mutex
	requests, maxRequests int
	read, maxRead         int64
}

func (l *limits) request() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.requests >= l.maxRequests {
		return false
	}
	l.requests++
	return true
}
func (l *limits) seal() { l.mu.Lock(); defer l.mu.Unlock(); l.maxRequests = l.requests }
func (l *limits) charge(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.read += int64(n)
	return l.read <= l.maxRead
}

type policyTransport struct {
	base         http.RoundTripper
	origin, path string
	allowed      *atomic.Bool
	limits       *limits
	calls        atomic.Int64
}

func (p *policyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	p.calls.Add(1)
	if r.Context().Err() != nil {
		return nil, r.Context().Err()
	}
	if !p.allowed.Load() || r.URL.Scheme+"://"+r.URL.Host != p.origin || !strings.HasPrefix(r.URL.Path, p.path+"/") || !p.limits.request() {
		return nil, errPolicy
	}
	response, err := p.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	if !p.allowed.Load() {
		response.Body.Close()
		return nil, errPolicy
	}
	response.Body = &policyBody{ReadCloser: response.Body, ctx: r.Context(), allowed: p.allowed, limits: p.limits}
	return response, nil
}

type policyBody struct {
	io.ReadCloser
	ctx     context.Context
	allowed *atomic.Bool
	limits  *limits
}

func (b *policyBody) Read(p []byte) (int, error) {
	if b.ctx.Err() != nil {
		return 0, b.ctx.Err()
	}
	if !b.allowed.Load() {
		return 0, errPolicy
	}
	n, err := b.ReadCloser.Read(p)
	if b.ctx.Err() != nil || !b.allowed.Load() || !b.limits.charge(n) {
		clear(p[:n])
		return 0, errPolicy
	}
	return n, err
}

type fixture struct {
	root, serverRoot, gitBackend string
	ids, latest, tokens          map[string]string
	formats                      map[string]format.ObjectFormat
	remote, trap                 *httptest.Server
	calls, wrongAuth, trapCalls  atomic.Int64
	mode                         atomic.Int32
	active                       atomic.Pointer[atomic.Bool]
	entered, canceled            chan struct{}
	signal                       atomic.Bool
}

func newFixture() *fixture {
	root, err := os.MkdirTemp("", "aim-go-git-v6-probe-*")
	need(err == nil, "temporary fixture")
	f := &fixture{root: root, serverRoot: filepath.Join(root, "upstream"), ids: map[string]string{}, latest: map[string]string{}, tokens: map[string]string{"a": "SYNTHETIC-TOKEN-A", "b": "SYNTHETIC-TOKEN-B", "c": "SYNTHETIC-TOKEN-C"}, formats: map[string]format.ObjectFormat{"a": format.SHA1, "b": format.SHA1, "c": format.SHA256}, entered: make(chan struct{}), canceled: make(chan struct{})}
	need(os.Mkdir(f.serverRoot, 0700) == nil, "server fixture directory")
	for _, name := range []string{"a", "b", "c"} {
		seed := filepath.Join(root, "seed-"+name)
		need(os.Mkdir(seed, 0700) == nil, "seed directory")
		objectFormat := "sha1"
		if name == "c" {
			objectFormat = "sha256"
		}
		bare := filepath.Join(f.serverRoot, name+".git")
		mustGit(seed, "init", "--bare", "--template=", "--object-format="+objectFormat, bare)
		mustGit(seed, "init", "--template=", "--object-format="+objectFormat, seed)
		mustGit(seed, "config", "user.name", "Fixture")
		mustGit(seed, "config", "user.email", "fixture@example.invalid")
		for _, version := range []string{"base", "pinned", "new"} {
			need(os.WriteFile(filepath.Join(seed, "evidence.txt"), []byte("synthetic-"+name+"-"+version+"\n"), 0600) == nil, "synthetic content")
			mustGit(seed, "add", "--", "evidence.txt")
			mustGit(seed, "commit", "-m", version)
			if version == "pinned" {
				f.ids[name] = mustGit(seed, "rev-parse", "HEAD")
			}
		}
		f.latest[name] = mustGit(seed, "rev-parse", "HEAD")
		mustGit(seed, "push", bare, "HEAD:refs/heads/main")
		mustGit(bare, "config", "http.receivepack", "false")
		mustGit(bare, "config", "http.getanyfile", "false")
		mustGit(bare, "config", "uploadpack.allowReachableSHA1InWant", "true")
	}
	f.gitBackend = filepath.Join(mustGit(root, "--exec-path"), "git-http-backend")
	f.trap = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.trapCalls.Add(1); w.WriteHeader(500) }))
	f.remote = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		name := strings.TrimSuffix(strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0], ".git")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("fixture:"+f.tokens[name]))
		if f.tokens[name] == "" || r.Header.Get("Authorization") != want {
			f.wrongAuth.Add(1)
			w.WriteHeader(403)
			return
		}
		switch f.mode.Load() {
		case 1:
			http.Redirect(w, r, f.trap.URL+"/must-not-receive-token", 302)
			return
		case 2:
			f.active.Load().Store(false)
		case 3:
			if f.signal.CompareAndSwap(false, true) {
				close(f.entered)
			}
			<-r.Context().Done()
			if f.signal.CompareAndSwap(true, false) {
				close(f.canceled)
			}
			return
		case 4:
			w.WriteHeader(500)
			io.WriteString(w, "SYNTHETIC-TOKEN-A must not be published")
			return
		}
		r.Header.Del("Authorization")
		(&cgi.Handler{Path: f.gitBackend, Root: "/", Dir: f.serverRoot, Env: append(gitEnv(f.serverRoot), "GIT_PROJECT_ROOT="+f.serverRoot, "GIT_HTTP_EXPORT_ALL=1", "GIT_PROTOCOL="+r.Header.Get("Git-Protocol"))}).ServeHTTP(w, r)
	}))
	return f
}
func (f *fixture) close() { f.remote.Close(); f.trap.Close(); os.RemoveAll(f.root) }
func (f *fixture) fetch(ctx context.Context, name, sha string, allowed *atomic.Bool, l *limits) (string, *git.Repository, error) {
	dir, err := os.MkdirTemp(f.root, "consumer-*")
	if err != nil {
		return "", nil, err
	}
	repo, err := git.PlainInit(dir, true, git.WithObjectFormat(f.formats[name]))
	if err != nil {
		return dir, nil, err
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{f.remote.URL + "/" + name + ".git"}})
	if err != nil {
		return dir, repo, err
	}
	c := &http.Client{Transport: f.remote.Client().Transport, Timeout: 10 * time.Second}
	p := &policyTransport{base: c.Transport, origin: f.remote.URL, path: "/" + name + ".git", allowed: allowed, limits: l}
	c.Transport = p
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	err = repo.FetchContext(ctx, &git.FetchOptions{RefSpecs: []config.RefSpec{config.RefSpec(sha + ":refs/heads/pinned")}, Depth: 1, Tags: plumbing.NoTags, ClientOptions: []client.Option{client.WithHTTPClient(c), client.WithHTTPAuth(&githttp.BasicAuth{Username: "fixture", Password: f.tokens[name]}), client.WithRedirectPolicy(client.NoFollowRedirects)}})
	// A successful library return is never authority to ignore an application fence.
	if err == nil && !allowed.Load() {
		err = errPolicy
	}
	return dir, repo, err
}
func permission() *atomic.Bool { p := new(atomic.Bool); p.Store(true); return p }
func budget() *limits          { return &limits{maxRequests: 128, maxRead: 16 << 20} }
func run() map[string]any {
	f := newFixture()
	defer f.close()
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	allowed := permission()
	f.active.Store(allowed)
	l := budget()
	type outcome struct {
		name, dir string
		repo      *git.Repository
		err       error
	}
	results := make(chan outcome, 3)
	for _, name := range []string{"a", "b", "c"} {
		go func(name string) {
			dir, repo, err := f.fetch(ctx, name, f.ids[name], allowed, l)
			results <- outcome{name, dir, repo, err}
		}(name)
	}
	for range 3 {
		result := <-results
		need(result.err == nil, "fixed SHA fetch with per-operation HTTP client")
		defer result.repo.Close()
		need(mustGit(result.dir, "show", f.ids[result.name]+":evidence.txt") == "synthetic-"+result.name+"-pinned", "CLI reads fixed non-tip object")
		need(mustGit(result.dir, "rev-parse", "--is-shallow-repository") == "true", "shallow marker understood by Git CLI")
		need(mustGit(result.dir, "rev-parse", "refs/heads/pinned") == f.ids[result.name] && f.ids[result.name] != f.latest[result.name], "frozen ref not current branch tip")
		_, err := command(result.dir, "fsck", "--connectivity-only")
		need(err == nil, "native Git object interoperability")
		cfg, err := os.ReadFile(filepath.Join(result.dir, "config"))
		need(err == nil, "read bare config")
		for _, token := range f.tokens {
			need(!bytes.Contains(cfg, []byte(token)), "upstream token absent from repository config")
		}
	}
	need(f.wrongAuth.Load() == 0, "concurrent credentials isolated")
	calls := f.calls.Load()
	l.seal()
	_, repo, err := f.fetch(ctx, "a", f.ids["a"], allowed, l)
	if repo != nil {
		repo.Close()
	}
	need(err != nil && f.calls.Load() == calls, "shared request budget cannot reset per source")
	allowed = permission()
	f.active.Store(allowed)
	f.mode.Store(1)
	before := f.calls.Load()
	_, repo, err = f.fetch(ctx, "a", f.ids["a"], allowed, budget())
	if repo != nil {
		repo.Close()
	}
	need(err != nil && f.calls.Load() == before+1 && f.trapCalls.Load() == 0, "redirect rejected before destination")
	allowed = permission()
	f.active.Store(allowed)
	f.mode.Store(2)
	before = f.calls.Load()
	_, repo, err = f.fetch(ctx, "a", f.ids["a"], allowed, budget())
	if repo != nil {
		repo.Close()
	}
	need(err != nil && f.calls.Load() == before+1, "revocation stops next request")
	allowed = permission()
	f.active.Store(allowed)
	f.mode.Store(0)
	tiny := budget()
	tiny.maxRead = 16
	_, repo, err = f.fetch(ctx, "a", f.ids["a"], allowed, tiny)
	if repo != nil {
		repo.Close()
	}
	need(err != nil, "body budget failure propagated")
	allowed = permission()
	f.active.Store(allowed)
	f.mode.Store(4)
	_, repo, err = f.fetch(ctx, "a", f.ids["a"], allowed, budget())
	if repo != nil {
		repo.Close()
	}
	need(err != nil, "upstream error observed")
	rawErrorHasSyntheticToken := strings.Contains(err.Error(), "SYNTHETIC-TOKEN-A")
	allowed = permission()
	f.active.Store(allowed)
	f.mode.Store(3)
	cancelCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, repo, err := f.fetch(cancelCtx, "a", f.ids["a"], allowed, budget())
		if repo != nil {
			repo.Close()
		}
		done <- err
	}()
	select {
	case <-f.entered:
	case <-time.After(5 * time.Second):
		panic("cancellation fixture never entered")
	}
	start := time.Now()
	cancel()
	select {
	case err = <-done:
		need(err != nil, "library canceled return")
	case <-time.After(3 * time.Second):
		panic("library cancellation deadline")
	}
	select {
	case <-f.canceled:
	case <-time.After(3 * time.Second):
		panic("upstream cancellation missing")
	}
	return map[string]any{"synthetic_only": true, "go_git_version": "v6.0.0-beta.1", "git_version": mustGit(f.root, "--version"), "sha1_fixed_non_tip_fetch": true, "sha256_fixed_non_tip_fetch": true, "concurrent_per_operation_clients": true, "wrong_credentials": f.wrongAuth.Load(), "shallow_cli_interoperability": true, "shared_request_budget": true, "redirect_target_requests": f.trapCalls.Load(), "revocation_stops_next_request": true, "body_limit_fails_fetch": true, "cancellation_observed_upstream": true, "cancellation_milliseconds": time.Since(start).Milliseconds(), "raw_upstream_error_contains_synthetic_token": rawErrorHasSyntheticToken, "production_adapter_tested": false}
}
func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "pinned go-git probe failed; raw provider/credential errors omitted")
			os.Exit(1)
		}
	}()
	need(json.NewEncoder(os.Stdout).Encode(run()) == nil, "write proof")
}
