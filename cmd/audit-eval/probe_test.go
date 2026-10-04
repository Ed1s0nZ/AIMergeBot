package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func captureProbe(t *testing.T, endpoint string) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	err = probeModel(context.Background(), endpoint, "private-fixture-key", "synthetic-model")
	os.Stdout = original
	writer.Close()
	raw, _ := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func TestProbeRedactsBodyCredentialsAndUnknownCodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-fixture-key" {
			t.Error("configured credential not used")
		}
		w.WriteHeader(401)
		io.WriteString(w, `{"error":{"code":"unknown-private-fixture-key","message":"private-fixture-key secret provider body"}}`)
	}))
	defer server.Close()
	output := captureProbe(t, server.URL)
	if !strings.Contains(output, `"http_status":401`) || strings.Contains(output, "private-fixture-key") || strings.Contains(output, "secret provider body") {
		t.Fatal("probe disclosure", output)
	}
}
func TestProbeDoesNotForwardCredentialOnRedirect(t *testing.T) {
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	output := captureProbe(t, server.URL)
	if forwarded.Load() != 0 || !strings.Contains(output, `"http_status":302`) {
		t.Fatal("credential redirect followed", output, forwarded.Load())
	}
}
