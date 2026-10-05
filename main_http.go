package main

import (
	"net/http"
	"time"
)

// Audits run asynchronously; HTTP connections need only serve bounded API
// requests and static assets, not wait for an entire model investigation.
func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
}
