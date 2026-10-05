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

func TestFollowupHTTPFixedVersionAndValidation(t *testing.T) {
	s, r, parent := followupFixture(t)
	_, token, err := s.Login(context.Background(), "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s, Runner: r}).Register(router)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	prefix := fmt.Sprintf("/api/v1/runs/%d", parent.ID)
	scope := request("GET", prefix+"/scope", "")
	var list RunScope
	if scope.Code != 200 || json.Unmarshal(scope.Body.Bytes(), &list) != nil || list.HeadSHA != "original-head" || len(list.Files) != 3 {
		t.Fatalf("scope status %d", scope.Code)
	}
	first := request("POST", prefix+"/followup", `{"files":["b.any"]}`)
	if first.Code != 202 {
		t.Fatalf("submit status %d", first.Code)
	}
	var response struct {
		ID      int64 `json:"id"`
		Created bool  `json:"created"`
	}
	if json.Unmarshal(first.Body.Bytes(), &response) != nil || !response.Created {
		t.Fatal("missing child")
	}
	duplicate := request("POST", prefix+"/followup", `{"files":["b.any"]}`)
	var repeated struct {
		ID      int64 `json:"id"`
		Created bool  `json:"created"`
	}
	json.Unmarshal(duplicate.Body.Bytes(), &repeated)
	if duplicate.Code != 202 || repeated.Created || repeated.ID != response.ID {
		t.Fatal("dedup failed")
	}
	for _, body := range []string{`{"files":[]}`, `{"files":["../bad"]}`, `{"files":["readme.md"]}`, `{"files":["not-changed.any"]}`} {
		if w := request("POST", prefix+"/followup", body); w.Code != 422 {
			t.Fatalf("invalid selection status %d", w.Code)
		}
	}
}
