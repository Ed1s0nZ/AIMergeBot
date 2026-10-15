package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGitCommandCancellationStopsHTTPHelper(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("process-group cancellation is implemented for Linux/macOS")
	}
	dir := t.TempDir()
	if _, err := gitCommand(context.Background(), dir, nil, "init", "--bare", "--template=", dir); err != nil {
		t.Fatal(err)
	}
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var startOnce, canceledOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(entered) })
		select {
		case <-r.Context().Done():
			canceledOnce.Do(func() { close(canceled) })
		case <-release:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	returned := false
	defer func() {
		cancel()
		server.CloseClientConnections()
		close(release)
		if !returned {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("command did not stop after fixture connection cleanup")
			}
		}
		server.Close()
	}()
	go func() {
		_, err := gitCommand(ctx, dir, nil, "fetch", "--no-tags", "--depth=1", server.URL+"/fixture.git", strings.Repeat("a", 40))
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		returned = true
		t.Fatalf("fetch ended before blocking fixture: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not reach fixture")
	}
	cancel()
	upstreamCanceled := false
	commandDone, requestDone := done, canceled
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
observe:
	for !returned || !upstreamCanceled {
		select {
		case err := <-commandDone:
			returned = true
			commandDone = nil
			if !errors.Is(err, context.Canceled) {
				t.Errorf("wrong cancellation classification: %v", err)
			}
		case <-requestDone:
			upstreamCanceled = true
			requestDone = nil
		case <-deadline.C:
			break observe
		}
	}
	t.Logf("command_returned=%v upstream_canceled=%v", returned, upstreamCanceled)
	if !returned || !upstreamCanceled {
		t.Error("cancellation did not promptly stop Git and its upstream request")
	}
}

func TestGitCommandPreCanceledMakesNoHTTPRequest(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gitCommand(ctx, t.TempDir(), nil, "ls-remote", server.URL+"/fixture.git")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled command returned %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 0 {
		t.Fatalf("pre-canceled command made %d requests", requests)
	}
}
