//go:build ignore

// Controlled SDK/wire experiment. No production repository reader is imported.
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/xanzy/go-gitlab"
)

var errWire = errors.New("controlled transport allowance unavailable")
var errObject = errors.New("controlled object identity unavailable")
var errProtocol = errors.New("controlled provider response unavailable")

func need(ok bool, label string) {
	if !ok {
		panic(label)
	}
}
func git(dir, input string, args ...string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "credential.helper="}, args...)...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0"}
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = time.Second
	raw, err := cmd.Output()
	need(err == nil, "synthetic Git command")
	return raw
}
func oidValid(id string) bool {
	if len(id) != 40 && len(id) != 64 || strings.Trim(id, "0") == "" {
		return false
	}
	for _, ch := range id {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
func objectMatches(id string, body []byte) bool {
	var h hash.Hash
	if len(id) == 40 {
		h = sha1.New()
	} else if len(id) == 64 {
		h = sha256.New()
	} else {
		return false
	}
	fmt.Fprintf(h, "blob %d\x00", len(body))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)) == id
}

type fixture struct {
	commit string
	nodes  []gitlab.TreeNode
	blobs  map[string][]byte
	byPath map[string]string
}

func createFixture(root, format string) fixture {
	dir := filepath.Join(root, format)
	need(os.Mkdir(dir, 0700) == nil && os.Mkdir(filepath.Join(dir, "pkg"), 0700) == nil, "fixture directory")
	git(dir, "", "init", "--quiet", "--template=", "--object-format="+format, dir)
	git(dir, "", "config", "user.name", "Fixture")
	git(dir, "", "config", "user.email", "fixture@example.invalid")
	for n := 0; n < 101; n++ {
		name := fmt.Sprintf("f%03d.txt", n)
		need(os.WriteFile(filepath.Join(dir, "pkg", name), []byte("synthetic-"+name+"\n"), 0600) == nil, "regular fixture file")
	}
	for name, body := range map[string][]byte{"raw.bin": {0xff, 0, 'x'}, "empty": {}, "limit.txt": bytes.Repeat([]byte("x"), 30), "executable": []byte("synthetic-executable\n")} {
		mode := os.FileMode(0600)
		if name == "executable" {
			mode = 0700
		}
		need(os.WriteFile(filepath.Join(dir, "pkg", name), body, mode) == nil, "special fixture file")
	}
	need(os.Symlink("f000.txt", filepath.Join(dir, "pkg", "link")) == nil, "synthetic symlink")
	git(dir, "", "add", "--", "pkg")
	git(dir, "", "commit", "--quiet", "-m", "synthetic")
	parent := strings.TrimSpace(string(git(dir, "", "rev-parse", "HEAD")))
	git(dir, "", "update-index", "--add", "--cacheinfo", "160000,"+parent+",pkg/dependency")
	git(dir, "", "commit", "--quiet", "-m", "synthetic-gitlink")
	f := fixture{commit: strings.TrimSpace(string(git(dir, "", "rev-parse", "HEAD"))), blobs: map[string][]byte{}, byPath: map[string]string{}}
	for _, row := range bytes.Split(git(dir, "", "ls-tree", "-z", "HEAD:pkg"), []byte{0}) {
		if len(row) == 0 {
			continue
		}
		parts := bytes.SplitN(row, []byte{'\t'}, 2)
		need(len(parts) == 2, "real ls-tree row")
		fields := strings.Fields(string(parts[0]))
		need(len(fields) == 3, "real ls-tree metadata")
		name := string(parts[1])
		node := gitlab.TreeNode{Mode: fields[0], Type: fields[1], ID: fields[2], Name: name, Path: "pkg/" + name}
		f.nodes = append(f.nodes, node)
		f.byPath[node.Path] = node.ID
		if node.Type == "blob" {
			f.blobs[node.ID] = git(dir, "", "cat-file", "blob", node.ID)
		}
	}
	return f
}

// Operation-wide synchronized reservations around the existing SDK. This
// experiment uses serial SDK reads/Close, not a production durable ledger.
type wireMeter struct {
	base                  http.RoundTripper
	mu                    sync.Mutex
	left, perBody         int64
	requests, maxRequests int
}
type wireBody struct {
	io.ReadCloser
	meter          *wireMeter
	reserved, used int64
	once           sync.Once
}

func (b *wireBody) Read(p []byte) (int, error) {
	p = p[:min(int64(len(p)), b.reserved-b.used)]
	if len(p) == 0 {
		return 0, errWire
	}
	n, err := b.ReadCloser.Read(p)
	b.used += int64(n)
	if b.used > b.meter.perBody {
		return n, errWire
	}
	return n, err
}
func (b *wireBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		b.meter.mu.Lock()
		b.meter.left += b.reserved - b.used
		b.meter.mu.Unlock()
	})
	return err
}
func (m *wireMeter) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	grant := m.perBody + 1
	if m.requests >= m.maxRequests || m.left < grant {
		m.mu.Unlock()
		return nil, errWire
	}
	m.requests++
	m.left -= grant
	m.mu.Unlock()
	response, err := m.base.RoundTrip(request)
	if err != nil {
		m.mu.Lock()
		m.left += grant
		m.mu.Unlock()
		return nil, err
	}
	response.Body = &wireBody{ReadCloser: response.Body, meter: m, reserved: grant}
	return response, nil
}
func rawBlob(ctx context.Context, client *gitlab.Client, project int, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !oidValid(id) {
		return nil, errObject
	}
	raw, _, err := client.Repositories.RawBlobContent(project, id, gitlab.WithContext(ctx))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errProtocol // Never wrap provider body or raw SDK error.
	}
	if !objectMatches(id, raw) {
		return nil, errObject
	}
	return raw, nil
}
func directory(ctx context.Context, client *gitlab.Client, project int, ref string) ([]*gitlab.TreeNode, error) {
	if !oidValid(ref) {
		return nil, errObject
	}
	var out []*gitlab.TreeNode
	seen := map[string]bool{}
	for page := 1; page <= 20; page++ {
		nodes, response, err := client.Repositories.ListTree(project, &gitlab.ListTreeOptions{Path: gitlab.String("pkg"), Ref: gitlab.String(ref), Recursive: gitlab.Bool(false), ListOptions: gitlab.ListOptions{Page: page, PerPage: 100}}, gitlab.WithContext(ctx))
		if err != nil || response == nil || len(nodes) > 100 {
			return nil, errProtocol
		}
		for _, node := range nodes {
			if node == nil || seen[node.Path] || node.Path != "pkg/"+node.Name || strings.Contains(node.Name, "/") || node.Name == "." || node.Name == ".." || path.Clean(node.Path) != node.Path || !utf8.ValidString(node.Path) || strings.IndexFunc(node.Path, unicode.IsControl) >= 0 || !oidValid(node.ID) || len(node.ID) != len(ref) {
				return nil, errObject
			}
			validMode := node.Type == "blob" && (node.Mode == "100644" || node.Mode == "100755" || node.Mode == "120000") || node.Type == "commit" && node.Mode == "160000" || node.Type == "tree" && node.Mode == "040000"
			if !validMode {
				return nil, errObject
			}
			seen[node.Path] = true
			out = append(out, node)
		}
		if response.NextPage == 0 {
			if len(nodes) == 100 {
				return nil, errProtocol // Full page without terminal proof is not absence.
			}
			return out, nil
		}
		if response.NextPage != page+1 {
			return nil, errProtocol
		}
	}
	return nil, errProtocol
}

func run() map[string]any {
	root, err := os.MkdirTemp("", "aim-gitlab-object-probe-")
	need(err == nil, "temporary root")
	defer os.RemoveAll(root)
	fixtures := map[int]fixture{101: createFixture(root, "sha1"), 202: createFixture(root, "sha256")}
	previousPath := os.Getenv("PATH")
	need(os.Setenv("PATH", filepath.Join(root, "no-executables")) == nil, "disable Git PATH after fixture generation")
	defer os.Setenv("PATH", previousPath)
	var mode atomic.Int32 // 1 corrupt raw, 2 oversized, 3 provider body, 4 missing page, 5 duplicate, 6 delayed, 7 invalid mode, 8 wrong format.
	var requests, wrong, rejectedFixedRef atomic.Int64
	started, stopped := make(chan struct{}, 1), make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.Header.Get("PRIVATE-TOKEN") != "SYNTHETIC-TOKEN" {
			wrong.Add(1)
			w.WriteHeader(403)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 6 || parts[0] != "api" || parts[1] != "v4" || parts[2] != "projects" || parts[4] != "repository" {
			wrong.Add(1)
			w.WriteHeader(404)
			return
		}
		project, _ := strconv.Atoi(parts[3])
		f, exists := fixtures[project]
		if !exists {
			w.WriteHeader(404)
			return
		}
		if parts[5] == "tree" && len(parts) == 6 {
			query := r.URL.Query()
			page, _ := strconv.Atoi(query.Get("page"))
			if query.Get("ref") != f.commit {
				if query.Get("ref") == strings.Repeat("f", len(f.commit)) {
					rejectedFixedRef.Add(1)
				} else {
					wrong.Add(1)
				}
				w.WriteHeader(404)
				return
			}
			if query.Get("path") != "pkg" || query.Get("per_page") != "100" || query.Get("recursive") != "false" || page < 1 || page > 2 {
				wrong.Add(1)
				w.WriteHeader(400)
				return
			}
			begin, end := (page-1)*100, min(page*100, len(f.nodes))
			nodes := append([]gitlab.TreeNode(nil), f.nodes[begin:end]...)
			if page == 1 && mode.Load() != 4 {
				w.Header().Set("X-Next-Page", "2")
			}
			if page == 2 && mode.Load() == 5 {
				nodes[0] = f.nodes[0]
			}
			if mode.Load() == 7 {
				nodes[0].Mode = "077777"
			}
			if mode.Load() == 8 {
				nodes[0].ID = strings.Repeat("f", 64)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nodes)
			return
		}
		if parts[5] == "blobs" && len(parts) == 8 && parts[7] == "raw" {
			raw, exists := f.blobs[parts[6]]
			if !exists {
				w.WriteHeader(404)
				return
			}
			switch mode.Load() {
			case 1:
				raw = append(append([]byte{}, raw...), 'x')
			case 2:
				raw = bytes.Repeat([]byte("x"), (256<<10)+1)
			case 3:
				w.WriteHeader(403)
				w.Write([]byte(`{"message":"SYNTHETIC-PROVIDER-SECRET"}`))
				return
			case 6:
				started <- struct{}{}
				<-r.Context().Done()
				stopped <- struct{}{}
				return
			}
			w.Write(raw)
			return
		}
		wrong.Add(1)
		w.WriteHeader(404)
	}))
	defer server.Close()
	newClient := func(limit, total int64, count int) (*gitlab.Client, *wireMeter) {
		meter := &wireMeter{base: server.Client().Transport, perBody: limit, left: total, maxRequests: count}
		client, err := gitlab.NewClient("SYNTHETIC-TOKEN", gitlab.WithBaseURL(server.URL+"/api/v4"), gitlab.WithoutRetries(), gitlab.WithHTTPClient(&http.Client{Transport: meter, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}))
		need(err == nil, "SDK configuration")
		return client, meter
	}
	client, _ := newClient(256<<10, 4<<20, 100)
	for project, f := range fixtures {
		nodes, err := directory(context.Background(), client, project, f.commit)
		need(err == nil && len(nodes) == len(f.nodes) && len(nodes) > 100, "fixed directory complete across pages")
		modes := map[string]bool{}
		for i, node := range nodes {
			need(*node == f.nodes[i], "metadata equals actual Git tree")
			modes[node.Mode] = true
		}
		need(modes["100644"] && modes["100755"] && modes["120000"] && modes["160000"], "all metadata modes preserved")
		for _, path := range []string{"pkg/raw.bin", "pkg/empty", "pkg/link", "pkg/executable"} {
			id := f.byPath[path]
			raw, err := rawBlob(context.Background(), client, project, id)
			need(err == nil && bytes.Equal(raw, f.blobs[id]), "raw bytes and OID equal actual Git")
		}
	}
	f := fixtures[101]
	id := f.byPath["pkg/limit.txt"]
	for _, fault := range []int32{1, 2, 3} {
		mode.Store(fault)
		raw, err := rawBlob(context.Background(), client, 101, id)
		need(err != nil && raw == nil && !strings.Contains(err.Error(), "SYNTHETIC"), "raw identity/limit/provider rejection is sanitized")
	}
	for _, fault := range []int32{4, 5, 7, 8} {
		mode.Store(fault)
		nodes, err := directory(context.Background(), client, 101, f.commit)
		need(err != nil && nodes == nil, "bad pagination or metadata is not complete")
	}
	mode.Store(0)
	nodes, err := directory(context.Background(), client, 101, strings.Repeat("f", len(f.commit)))
	need(err != nil && nodes == nil && rejectedFixedRef.Load() == 1, "missing fixed ref is not replaced by default branch")
	limited, meter := newClient(32, 66, 100)
	before := requests.Load()
	for n := 0; n < 3; n++ {
		_, err := rawBlob(context.Background(), limited, 101, id)
		need(n < 2 && err == nil || n == 2 && err != nil, "shared bytes do not reset per request")
	}
	need(requests.Load()-before == 2 && meter.left == 6, "shared budget prevents next dispatch")
	requestLimited, _ := newClient(256<<10, 4<<20, 1)
	before = requests.Load()
	_, firstErr := rawBlob(context.Background(), requestLimited, 101, id)
	_, nextErr := rawBlob(context.Background(), requestLimited, 101, id)
	need(firstErr == nil && nextErr != nil && requests.Load()-before == 1, "request cap prevents next dispatch")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before = requests.Load()
	_, err = rawBlob(ctx, client, 101, id)
	need(errors.Is(err, context.Canceled) && requests.Load() == before, "pre-cancel sends no request")
	_, err = rawBlob(context.Background(), client, 101, "invalid")
	need(errors.Is(err, errObject) && requests.Load() == before, "invalid OID sends no request")
	mode.Store(6)
	ctx, cancel = context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := rawBlob(ctx, client, 101, id); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		panic("delayed request never started")
	}
	cancel()
	select {
	case err = <-done:
		need(errors.Is(err, context.Canceled), "mid-request cancellation is observed")
	case <-time.After(3 * time.Second):
		panic("canceled request did not return")
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		panic("upstream cancellation not observed")
	}
	need(wrong.Load() == 0, "all actual SDK routes/auth/refs are expected")
	return map[string]any{"synthetic_only": true, "sdk": "go-gitlab/v0.90.0", "sha1_sha256_fixed_raw_and_tree": true, "directory_entries_per_format": len(f.nodes), "metadata_modes": 4, "raw_empty_non_utf8_symlink_preserved": true, "hash_oversize_provider_errors_rejected": true, "missing_duplicate_pages_rejected": true, "invalid_mode_and_object_format_rejected": true, "missing_fixed_ref_no_branch_fallback": true, "api_reads_with_git_path_disabled": true, "shared_bytes_next_dispatch_stopped": true, "request_cap_next_dispatch_stopped": true, "pre_canceled_zero_requests": true, "mid_request_cancellation_observed": true, "real_gitlab_server_tested": false, "production_adapter_tested": false}
}
func main() {
	defer func() {
		if problem := recover(); problem != nil {
			fmt.Fprintln(os.Stderr, "GitLab protocol probe failed:", problem)
			os.Exit(1)
		}
	}()
	need(json.NewEncoder(os.Stdout).Encode(run()) == nil, "JSON output")
}
