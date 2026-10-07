package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSlackRunStatusRequiresCurrentBindingFullSnapshotAndScope(t *testing.T) {
	for _, change := range []string{"valid", "target", "source", "context", "disabled-user", "disabled-project", "disabled-source", "channel-scope", "binding"} {
		t.Run(change, func(t *testing.T) {
			s, admin, member, snap := contextFixture(t)
			ctx := context.Background()
			if err := s.SaveProject(ctx, Project{ID: 3, Name: "source", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			snap.SourceProjectID = 3
			if err := s.SetProjectMember(ctx, 3, member.ID, "viewer", admin.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.SetProjectMember(ctx, 2, member.ID, "viewer", admin.ID); err != nil {
				t.Fatal(err)
			}
			run, _, err := s.Enqueue(ctx, snap, admin.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			zero := int64(0)
			secret := "fixture-signing-secret"
			channel, err := s.SaveIntegration(ctx, 0, admin.ID, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: secret, SlackAppID: "A1", SlackWorkspaceID: "T1"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`INSERT INTO platform_bot_bindings(integration_id,workspace_id,external_user_id,user_id,created_at) VALUES(?,'T1','U1',?,?)`, channel.ID, member.ID, now()); err != nil {
				t.Fatal(err)
			}
			var mutation string
			var args []any
			switch change {
			case "target":
				mutation = `DELETE FROM platform_project_members WHERE project_id=1 AND user_id=?`
				args = []any{member.ID}
			case "context":
				mutation = `DELETE FROM platform_project_members WHERE project_id=2 AND user_id=?`
				args = []any{member.ID}
			case "source":
				mutation = `DELETE FROM platform_project_members WHERE project_id=3 AND user_id=?`
				args = []any{member.ID}
			case "disabled-source":
				mutation = `UPDATE platform_projects SET enabled=0 WHERE id=3`
			case "disabled-user":
				mutation = `UPDATE platform_users SET disabled=1 WHERE id=?`
				args = []any{member.ID}
			case "disabled-project":
				mutation = `UPDATE platform_projects SET enabled=0 WHERE id=2`
			case "channel-scope":
				mutation = `UPDATE platform_integrations SET project_ids='[2]' WHERE id=?`
				args = []any{channel.ID}
			case "binding":
				mutation = `DELETE FROM platform_bot_bindings WHERE user_id=?`
				args = []any{member.ID}
			}
			if mutation != "" {
				if _, err := s.DB.Exec(mutation, args...); err != nil {
					t.Fatal(err)
				}
			}
			at := time.Unix(1800000000, 0)
			stamp := strconv.FormatInt(at.Unix(), 10)
			body := []byte(fmt.Sprintf("api_app_id=A1&team_id=T1&user_id=U1&text=status+%d", run))
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte("v0:" + stamp + ":"))
			mac.Write(body)
			signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
			out, err := s.slackRunStatus(ctx, channel.ID, channel.Revision, stamp, signature, body, at)
			if change != "valid" {
				if err == nil || out.ID != 0 {
					t.Fatal("unauthorized status exposed", out, err)
				}
			} else if err != nil || out.ID != run || out.HeadSHA != snap.HeadSHA || out.Status != "pending" {
				t.Fatal(out, err)
			}
			if change == "valid" {
				if _, err := s.slackRunStatus(ctx, channel.ID, channel.Revision, stamp, signature, body, at); err == nil {
					t.Fatal("status replay accepted")
				}
			}
			// A fresh timestamp avoids reusing the successful internal receipt.
			stamp = strconv.FormatInt(time.Now().Unix(), 10)
			mac = hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte("v0:" + stamp + ":"))
			mac.Write(body)
			signature = "v0=" + hex.EncodeToString(mac.Sum(nil))
			router := gin.New()
			(&HTTP{Store: s}).Register(router)
			request := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/bot-callbacks/slack/%d/%d/command", channel.ID, channel.Revision), strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("X-Slack-Request-Timestamp", stamp)
			request.Header.Set("X-Slack-Signature", signature)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if change == "valid" {
				if response.Code != 200 || !strings.Contains(response.Body.String(), snap.HeadSHA) || !strings.Contains(response.Body.String(), `"response_type":"ephemeral"`) {
					t.Fatal(response.Code, response.Body.String())
				}
			} else if response.Code != 403 || response.Body.String() != `{"error":"callback rejected"}` {
				t.Fatal("HTTP disclosed unauthorized status", response.Code, response.Body.String())
			}
		})
	}
}
