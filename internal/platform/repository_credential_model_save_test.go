package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentialModelOnlySaveKeepsReadersActiveBeforePublish(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	signal := sync.OnceFunc(func() { close(entered) })
	var block, requests atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Store(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"size": 6, "content": base64.StdEncoding.EncodeToString([]byte("source"))})
	}))
	defer server.Close()
	svc, s := activationFixture(t, credentialActivationOptions{beforePublish: func() error {
		if block.Load() {
			signal()
			<-release
		}
		return nil
	}})
	cfg := svc.Snapshot()
	cfg.GitLab.URL = server.URL
	if err := svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	operation, _, err := svc.legacyOperationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := svc.Snapshot()
	generation := activationGeneration(t, s)
	next := svc.Snapshot()
	next.OpenAI.Model = "new-model-only"
	block.Store(true)
	var saveErr error
	go func() { saveErr = svc.Save(next); close(finished) }()
	defer func() { unblock(); <-finished }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("model save never reached publication boundary")
	}
	if activationGeneration(t, s) != generation {
		t.Error("model save rotated repository credentials")
	}
	if svc.Snapshot().OpenAI.Model != before.OpenAI.Model {
		t.Error("model settings published before the atomic boundary")
	}
	if err := checkRepositoryFence(operation); err != nil {
		t.Error("model-only save revoked an existing operation", err)
	}
	body, err := (&DynamicRepository{Settings: svc}).ReadFile(operation, Snapshot{SourceProjectID: 1, HeadSHA: strings.Repeat("a", 40)}, "main.go", false)
	if err != nil || body != "source" || !requests.Load() {
		t.Error("model-only save stopped the next repository HTTP request", err)
	}
	unblock()
	<-finished
	if saveErr != nil || svc.Snapshot().OpenAI.Model != next.OpenAI.Model || activationGeneration(t, s) != generation || checkRepositoryFence(operation) != nil {
		t.Error("model-only publication did not preserve credential identity", saveErr)
	}
}

func TestCredentialModelOnlySaveFailureRemainsRecoverable(t *testing.T) {
	for _, mode := range []string{"before_replace", "after_replace", "after_commit"} {
		t.Run(mode, func(t *testing.T) {
			var inject atomic.Bool
			options := credentialActivationOptions{beforePublish: func() error {
				if inject.Load() && mode == "after_commit" {
					return errors.New("PRIVATE-PUBLISH-FAILURE")
				}
				return nil
			}}
			options.persist = func(path string, raw []byte) (bool, error) {
				if inject.Load() && mode == "before_replace" {
					return false, errors.New("PRIVATE-FILE-FAILURE")
				}
				replaced, err := persistSettingsFile(path, raw, true)
				if err == nil && inject.Load() && mode == "after_replace" {
					return true, errors.New("PRIVATE-DURABILITY-FAILURE")
				}
				return replaced, err
			}
			svc, s := activationFixture(t, options)
			ctx := context.Background()
			operation, _, err := svc.legacyOperationContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			before := svc.Snapshot()
			originalFile, err := os.ReadFile(svc.path)
			if err != nil {
				t.Fatal(err)
			}
			generation := activationGeneration(t, s)
			next := svc.Snapshot()
			next.OpenAI.Model = "new-model-only"
			inject.Store(true)
			if err = svc.Save(next); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("failure accepted or leaked", err)
			}
			if activationGeneration(t, s) != generation || svc.Snapshot().OpenAI.Model != before.OpenAI.Model || svc.Snapshot().Revision != before.Revision {
				t.Fatal("failed model save changed credential identity or live config")
			}
			raw, err := os.ReadFile(svc.path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "before_replace" {
				if string(raw) != string(originalFile) || checkRepositoryFence(operation) != nil {
					t.Fatal("definite model file failure interrupted credentials")
				}
				return
			}
			if checkRepositoryFence(operation) == nil || !strings.Contains(string(raw), "new-model-only") {
				t.Fatal("uncertain model save reopened gate or discarded durable file")
			}
			if err = svc.Save(before); !errors.Is(err, errCredentialActivationPending) {
				t.Fatal("pending model save allowed file overwrite", err)
			}
			after, _ := os.ReadFile(svc.path)
			if string(after) != string(raw) {
				t.Fatal("pending model file overwritten")
			}
		})
	}
}
