package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type runtimeFenceReader struct {
	read   func([]byte) (int, error)
	closed bool
}

func (r *runtimeFenceReader) Read(p []byte) (int, error) { return r.read(p) }
func (r *runtimeFenceReader) Close() error               { r.closed = true; return nil }

func TestRepositoryFenceComposesAndDiscardsFilledBody(t *testing.T) {
	var parent, child int
	ctx := withRepositoryFence(context.Background(), func(context.Context) error { parent++; return errors.New("PRIVATE-AUTH-ERROR") })
	ctx = withRepositoryFence(ctx, func(context.Context) error { child++; return nil })
	if err := checkRepositoryFence(ctx); !errors.Is(err, ErrRepositoryUnavailable) || strings.Contains(err.Error(), "PRIVATE") || parent != 1 || child != 0 {
		t.Fatal("child replaced parent or leaked authorization error", err)
	}
	allowed := true
	ctx = withRepositoryFence(context.Background(), func(context.Context) error {
		if !allowed {
			return ErrConflict
		}
		return nil
	})
	raw := &runtimeFenceReader{read: func(p []byte) (int, error) {
		n := copy(p, "PRIVATE-RESPONSE")
		allowed = false
		return n, io.EOF
	}}
	body := &repositoryFencedBody{ReadCloser: raw, ctx: ctx}
	buffer := make([]byte, 64)
	if n, err := body.Read(buffer); n != 0 || !errors.Is(err, ErrRepositoryUnavailable) || !raw.closed || strings.Contains(string(buffer), "PRIVATE") {
		t.Fatal("revoked body exposed filled buffer", n, err)
	}
}

func TestDynamicRepositoryRetainsParentCredentialFence(t *testing.T) {
	var requests atomic.Int32
	var tokenMu sync.Mutex
	tokens := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		tokenMu.Lock()
		tokens = append(tokens, r.Header.Get("PRIVATE-TOKEN"))
		tokenMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"size": 6, "content": base64.StdEncoding.EncodeToString([]byte("source"))})
	}))
	defer server.Close()
	svc, _ := activationFixture(t, credentialActivationOptions{})
	cfg := svc.Snapshot()
	cfg.GitLab.URL = server.URL
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	old, _, err := svc.legacyOperationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dynamic := &DynamicRepository{Settings: svc}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, HeadSHA: strings.Repeat("a", 40)}
	if body, err := dynamic.ReadFile(old, snap, "main.go", false); err != nil || body != "source" || requests.Load() != 1 {
		t.Fatal("valid dynamic read failed", err)
	}
	cfg = svc.Snapshot()
	cfg.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if body, err := dynamic.ReadFile(old, snap, "main.go", false); err == nil || body != "" || requests.Load() != 1 {
		t.Fatal("old operation adopted new token", err)
	}
	if body, err := dynamic.ReadFile(ctx, snap, "main.go", false); err != nil || body != "source" || requests.Load() != 2 {
		t.Fatal("fresh operation cannot use new credentials", err)
	}
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if len(tokens) != 2 || tokens[0] != "PRIVATE-A" || tokens[1] != "PRIVATE-B" {
		t.Fatal("unexpected token selection")
	}
}

func TestDynamicRepositoryDiscardsResponseAfterCredentialRotation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"size": 14, "content": base64.StdEncoding.EncodeToString([]byte("PRIVATE-SOURCE"))})
	}))
	defer server.Close()
	defer unblock()
	svc, _ := activationFixture(t, credentialActivationOptions{})
	cfg := svc.Snapshot()
	cfg.GitLab.URL = server.URL
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	done := make(chan struct{})
	var body string
	var readErr error
	go func() {
		body, readErr = (&DynamicRepository{Settings: svc}).ReadFile(ctx, Snapshot{SourceProjectID: 1, HeadSHA: strings.Repeat("a", 40)}, "main.go", false)
		close(done)
	}()
	defer func() { unblock(); <-done }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("request did not arrive")
	}
	cfg = svc.Snapshot()
	cfg.GitLab.Token = "PRIVATE-B"
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	unblock()
	<-done
	if readErr == nil || body != "" || requests.Load() != 1 {
		t.Fatal("in-flight revoked source consumed", readErr)
	}
}

func TestRepositoryFenceGuardsGitProcessAndCache(t *testing.T) {
	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "ran")
			if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n: > \"$FENCE_TEST_MARKER\"\nprintf PRIVATE-GIT\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			calls := 0
			ctx := withRepositoryFence(context.Background(), func(context.Context) error {
				calls++
				if mode == "after" && calls == 1 {
					return nil
				}
				return ErrConflict
			})
			if out, err := gitCommand(ctx, dir, []string{"FENCE_TEST_MARKER=" + marker}, "version"); out != "" || !errors.Is(err, ErrRepositoryUnavailable) {
				t.Fatal("Git output survived revoked operation", err)
			}
			_, err := os.Stat(marker)
			if (mode == "before" && !os.IsNotExist(err)) || (mode == "after" && err != nil) {
				t.Fatal("unexpected Git process count", err)
			}
		})
	}
	g := &GitRepository{Directory: "synthetic", pathCache: []gitPathCacheEntry{{directory: "synthetic", sha: "head", paths: []string{"PRIVATE-PATH"}}}}
	ctx := withRepositoryFence(context.Background(), func(context.Context) error { return ErrConflict })
	if paths, err := g.cachedPaths(ctx, "head"); len(paths) != 0 || !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("cache bypassed authorization", err)
	}
}
