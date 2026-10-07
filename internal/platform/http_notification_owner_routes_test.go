package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationOwnerRouteHTTPContractAndAuthorization(t *testing.T) {
	s, _, _, owner, v := notificationOwnerFixture(t, "instant")
	ctx := context.Background()
	_, adminToken, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	_, memberToken, err := s.Login(ctx, "owner", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	request := func(method, path, token, body, origin string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	path := fmt.Sprintf("/api/v1/integrations/%d", v.ID)
	input := IntegrationInput{Integration: v, ExpectedRevision: &v.Revision}
	body := func(in IntegrationInput) string {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	request("GET", "/api/v1/integrations", "", "", "", 401)
	request("GET", "/api/v1/integrations", memberToken, "", "", 403)
	request("PATCH", path, memberToken, body(input), "", 403)
	request("PATCH", path, adminToken, body(input), "https://attacker.example", 403)
	invalid := input
	invalid.OwnerIDs = []int64{owner.ID, owner.ID}
	request("PATCH", path, adminToken, body(invalid), "", 400)
	invalid = input
	invalid.Events = []string{"run.completed"}
	request("PATCH", path, adminToken, body(invalid), "", 400)
	read := request("GET", "/api/v1/integrations", adminToken, "", "", 200)
	if strings.Contains(read.Body.String(), "receiver.example") || !strings.Contains(read.Body.String(), fmt.Sprintf(`"owner_ids":[%d]`, owner.ID)) || read.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(read.Header(), read.Body.String())
	}
	// An old client's omitted field preserves the route.
	raw := body(input)
	var m map[string]any
	if err = json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "owner_ids")
	b, _ := json.Marshal(m)
	saved := request("PATCH", path, adminToken, string(b), "", 200)
	var got Integration
	if err = json.Unmarshal(saved.Body.Bytes(), &got); err != nil || len(got.OwnerIDs) != 1 {
		t.Fatal(got, err)
	}
	request("PATCH", path, adminToken, raw, "", 409)
	input.ExpectedRevision = &got.Revision
	input.OwnerIDs = []int64{}
	cleared := request("PATCH", path, adminToken, body(input), "", 200)
	if !strings.Contains(cleared.Body.String(), `"owner_ids":[]`) {
		t.Fatal(cleared.Body.String())
	}
}
