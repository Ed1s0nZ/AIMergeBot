package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPServerBoundsSlowBodyAndRemainsAvailable(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if server.ReadTimeout <= 0 || server.WriteTimeout <= server.ReadTimeout || server.ReadHeaderTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Fatal("unbounded request lifecycle")
	}
	// Exercise the same net/http deadline with a shorter observation period.
	server.ReadTimeout = 40 * time.Millisecond
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve(listener)
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(connection, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\nx"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatal("slow body not terminated by server", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		t.Fatal("body read was not bounded", response.StatusCode)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err = client.Post("http://"+listener.Addr().String(), "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal("server unavailable after timeout", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal(response.StatusCode)
	}
}
