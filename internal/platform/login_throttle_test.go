package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginThrottleProtectsAccountAcrossSourcesAndSharedEgress(t *testing.T) {
	h := &HTTP{}
	now := time.Now()
	for i := 0; i < 10; i++ {
		if h.admitLogin("shared", "target", true, now) != 0 {
			t.Fatal("early account throttle")
		}
	}
	if h.admitLogin("shared", "colleague", true, now) != 0 {
		t.Fatal("shared egress locked a different account")
	}
	h.loginSucceeded("shared", "colleague")
	if h.admitLogin("shared", "target", true, now) == 0 {
		t.Fatal("colleague success cleared target protection")
	}
	for i := 0; i < 30; i++ {
		if h.admitLogin(fmt.Sprint(i), "distributed-target", true, now) != 0 {
			t.Fatal("early account-wide throttle")
		}
	}
	if h.admitLogin("new-source", "distributed-target", true, now) == 0 {
		t.Fatal("source rotation bypassed account-wide protection")
	}
	if h.admitLogin("shared", "target", true, now.Add(loginWindow)) != 0 {
		t.Fatal("expired account protection did not recover")
	}
}

func TestLoginThrottleBoundsMemoryAndAtomicAdmission(t *testing.T) {
	h := &HTTP{}
	now := time.Now()
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h.admitLogin("shared", "", false, now) == 0 {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 100 {
		t.Fatalf("aggregate admission: %d", admitted.Load())
	}
	h.loginSucceeded("shared", "valid")
	if h.admitLogin("shared", "", false, now) == 0 {
		t.Fatal("success reset aggregate protection")
	}
	for i := 1; i < maxLoginBuckets; i++ {
		if h.admitLogin(fmt.Sprint(i), "", false, now) != 0 {
			t.Fatal("early capacity rejection")
		}
	}
	if h.admitLogin("over-capacity", "", false, now) == 0 || len(h.loginAttempts) != maxLoginBuckets {
		t.Fatal("registry overflow or active bucket eviction")
	}
	if h.admitLogin("after-expiry", "", false, now.Add(loginWindow)) != 0 || len(h.loginAttempts) != 1 {
		t.Fatal("registry failed to recover after expiry")
	}
}

func TestLoginHTTPRespectsProxyTrustAndSuccessDoesNotBypassThrottle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, trusted := range []bool{false, true} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			s := testStore(t)
			if err := s.Bootstrap(context.Background(), "admin", "a-long-password"); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			var proxies []string
			if trusted {
				proxies = []string{"198.51.100.9"}
			}
			if err := router.SetTrustedProxies(proxies); err != nil {
				t.Fatal(err)
			}
			h := &HTTP{Store: s}
			h.Register(router)
			login := func(username, password, forwarded string) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)))
				req.RemoteAddr = "198.51.100.9:12345"
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Forwarded-For", forwarded)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w
			}
			for i := 0; i < 10; i++ {
				if w := login("target", "wrong", "203.0.113.1"); w.Code != http.StatusUnauthorized {
					t.Fatalf("early rejection: %d", w.Code)
				}
			}
			if w := login("admin", "a-long-password", "203.0.113.1"); w.Code != http.StatusOK {
				t.Fatal("colleague blocked on shared egress", w.Code)
			}
			if w := login("target", "wrong", "203.0.113.1"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
				t.Fatal("valid account cleared target limit or missing retry information")
			}
			w := login("target", "wrong", "203.0.113.2")
			want := http.StatusTooManyRequests
			if trusted {
				want = http.StatusUnauthorized
			}
			if w.Code != want {
				t.Fatalf("forwarded client trust mismatch: got%d want%d", w.Code, want)
			}
		})
	}
}

func TestMalformedLoginRequestsConsumeAggregateQuota(t *testing.T) {
	h := &HTTP{}
	r := gin.New()
	r.SetTrustedProxies(nil)
	h.Register(r)
	for i := 0; i < 101; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := 400
		if i == 100 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("malformed request%d: got%d want%d", i, w.Code, want)
		}
	}
}
