package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBotBindingHTTPAuthenticatedOwnerAndOrigin(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser(ctx, "other", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: "private-secret", SlackAppID: "A1", SlackWorkspaceID: "T1"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	_, otherToken, err := s.Login(ctx, "other", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s}).Register(router)
	path := fmt.Sprintf("/api/v1/bot-bindings/%d", channel.ID)
	body := fmt.Sprintf(`{"expected_revision":%d,"user_id":%d}`, channel.Revision, other.ID)
	request := func(method, path, token, body, origin string) *httptest.ResponseRecorder {
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
		return res
	}
	if res := request("POST", path+"/challenge", "", body, ""); res.Code != 401 {
		t.Fatal(res.Code)
	}
	if res := request("POST", path+"/challenge", token, body, "https://attacker.example"); res.Code != 403 {
		t.Fatal(res.Code)
	}
	if res := request("POST", path+"/challenge", token, `{}`, ""); res.Code != 400 {
		t.Fatal(res.Code)
	}
	res := request("POST", path+"/challenge", token, body, "")
	if res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil || !bindingToken.MatchString(out.Token) {
		t.Fatal(out, err)
	}
	if res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), "private-secret") {
		t.Fatal("unsafe response")
	}
	var actor int64
	if err := s.DB.QueryRow(`SELECT user_id FROM platform_bot_binding_challenges`).Scan(&actor); err != nil || actor != 1 {
		t.Fatal("actor override", actor, err)
	}
	if res := request("DELETE", path, otherToken, "", ""); res.Code != 200 {
		t.Fatal(res.Code)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_binding_challenges`).Scan(&count); err != nil || count != 1 {
		t.Fatal("other user revoked challenge", count, err)
	}
	if res := request("DELETE", path, token, "", "https://attacker.example"); res.Code != 403 {
		t.Fatal(res.Code)
	}
	if res := request("DELETE", path, token, "", ""); res.Code != 200 || !strings.Contains(res.Body.String(), `"revoked":true`) {
		t.Fatal(res.Code, res.Body.String())
	}
	if res := request("DELETE", path, token, "", ""); res.Code != 200 || !strings.Contains(res.Body.String(), `"revoked":false`) {
		t.Fatal(res.Code, res.Body.String())
	}
	res = request("POST", path+"/challenge", token, body, "")
	if res.Code != 201 || json.Unmarshal(res.Body.Bytes(), &out) != nil {
		t.Fatal(res.Code, res.Body.String())
	}
	callbackPath := fmt.Sprintf("/api/v1/bot-callbacks/slack/%d/%d/bind", channel.ID, channel.Revision)
	callbackBody := "api_app_id=A1&team_id=T1&user_id=U1&text=bind+" + out.Token
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("private-secret"))
	mac.Write([]byte("v0:" + stamp + ":" + callbackBody))
	signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
	callback := func(signature, contentType, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", callbackPath, strings.NewReader(payload))
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-Slack-Request-Timestamp", stamp)
		req.Header.Set("X-Slack-Signature", signature)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	if res := callback("invalid", "application/x-www-form-urlencoded", callbackBody); res.Code != 403 {
		t.Fatal(res.Code)
	}
	if res := callback(signature, "application/json", callbackBody); res.Code != 400 {
		t.Fatal(res.Code)
	}
	if res := callback(signature, "application/x-www-form-urlencoded", strings.Repeat("x", 65537)); res.Code != 400 {
		t.Fatal(res.Code)
	}
	if res := callback(signature, "application/x-www-form-urlencoded", callbackBody); res.Code != 200 || !strings.Contains(res.Body.String(), `"response_type":"ephemeral"`) || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(res.Code, res.Body.String())
	}
	if res := callback(signature, "application/x-www-form-urlencoded", callbackBody); res.Code != 403 {
		t.Fatal("callback replay", res.Code)
	}
	if err := s.DB.QueryRow(`SELECT user_id FROM platform_bot_bindings`).Scan(&actor); err != nil || actor != 1 {
		t.Fatal(actor, err)
	}
}
