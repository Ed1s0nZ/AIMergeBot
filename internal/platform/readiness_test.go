package platform

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestReadinessRequiresCurrentLiveWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := testStore(t)
	r := &Runner{Store: s}
	h := &HTTP{Store: s, Runner: r}
	engine := gin.New()
	engine.GET("/readyz", h.ready)
	check := func(want int, body string) {
		t.Helper()
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil))
		if response.Code != want || response.Body.String() != body || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected public readiness: %d %s", response.Code, response.Body.String())
		}
	}
	fail := func() { check(503, `{"status":"unavailable"}`) }
	fail()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.state.Store(&workerRunState{ctx: ctx, owner: "private-fixture-owner"})
	fail()
	if err := s.AcquireWorkerInstance(ctx, "private-fixture-owner"); err != nil {
		t.Fatal(err)
	}
	check(200, `{"status":"ready"}`)
	requestCtx, requestCancel := context.WithCancel(context.Background())
	requestCancel()
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/readyz", nil).WithContext(requestCtx))
	if response.Code != 503 {
		t.Fatal("cancelled readiness request accepted")
	}
	expireInstance(t, s)
	fail()
	if _, err := s.DB.Exec(`UPDATE platform_worker_instance SET owner=?,lease_until=?`, "replacement", time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	fail()
	if _, err := s.DB.Exec(`UPDATE platform_worker_instance SET owner=?`, "private-fixture-owner"); err != nil {
		t.Fatal(err)
	}
	check(200, `{"status":"ready"}`)
	cancel()
	fail()
	r.state.Store(&workerRunState{ctx: context.Background(), owner: "private-fixture-owner"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	fail()
	h.Runner = nil
	fail()
	h.Store = nil
	fail()
}
