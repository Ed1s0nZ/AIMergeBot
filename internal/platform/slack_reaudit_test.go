package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSlackReauditAtomicPinnedAdmissionAndAuthorization(t *testing.T) {
	for _, change := range []string{"valid", "quota", "history", "viewer", "head", "legacy", "active", "target", "source", "context", "disabled-user", "disabled-project", "disabled-source", "channel-scope", "binding"} {
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
			if _, err := s.DB.Exec(`UPDATE platform_runs SET status='failed' WHERE id=?`, run); err != nil {
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
			case "viewer":
				mutation = `UPDATE platform_project_members SET role='viewer' WHERE project_id=1 AND user_id=?`
				args = []any{member.ID}
			case "legacy":
				mutation = `UPDATE platform_runs SET policy_version='legacy' WHERE id=?`
				args = []any{run}
			case "active":
				mutation = `UPDATE platform_runs SET status='running' WHERE id=?`
				args = []any{run}
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
			if change == "quota" {
				busy := snap
				busy.MRIID = 2
				if _, _, err := s.Enqueue(ctx, busy, admin.ID, false); err != nil {
					t.Fatal(err)
				}
				bindTestQuotas(s, AuditQuotas{OutstandingGlobal: 1})
			}
			if change == "history" {
				if _, err := s.DB.Exec(`CREATE TRIGGER fixture_reject_reaudit_history BEFORE INSERT ON platform_events WHEN NEW.action='bot.reaudit.requested' BEGIN SELECT RAISE(ABORT,'fixture history failure'); END`); err != nil {
					t.Fatal(err)
				}
			}

			at := time.Unix(1800000000, 0)
			head := snap.HeadSHA
			if change == "head" {
				head = strings.Repeat("d", 40)
			}
			body := []byte(fmt.Sprintf("api_app_id=A1&team_id=T1&user_id=U1&text=reaudit+%d+%s", run, head))
			call := func(at time.Time) (int64, bool, error) {
				stamp := strconv.FormatInt(at.Unix(), 10)
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write([]byte("v0:" + stamp + ":"))
				mac.Write(body)
				return s.slackReaudit(ctx, channel.ID, channel.Revision, stamp, "v0="+hex.EncodeToString(mac.Sum(nil)), body, at)
			}
			id, created, err := call(at)
			if change != "valid" {
				if err == nil || id != 0 || created {
					t.Fatal("unauthorized reaudit", id, created, err)
				}
				var count int
				if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_callback_receipts`).Scan(&count); err != nil || count != 0 {
					t.Fatal("rejected callback consumed", count, err)
				}
				want := 1
				if change == "quota" {
					want = 2
				}
				if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != want {
					t.Fatal("rejected callback admitted run", count, err)
				}
			} else {
				if err != nil || !created || id <= 0 || id == run {
					t.Fatal(id, created, err)
				}
				admitted, err := s.Run(ctx, id)
				if err != nil || admitted.HeadSHA != snap.HeadSHA || admitted.BaseSHA != snap.BaseSHA || admitted.RequestedBy != member.ID || admitted.Status != "pending" {
					t.Fatal(admitted, err)
				}
				if _, _, err := call(at); err == nil {
					t.Fatal("reaudit replay")
				}
				duplicate, created, err := call(at.Add(time.Second))
				if err != nil || created || duplicate != id {
					t.Fatal("duplicate active run", duplicate, created, err)
				}
			}
			stamp := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte("v0:" + stamp + ":"))
			mac.Write(body)
			signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
			httpCall := func(runner *Runner, signature string) *httptest.ResponseRecorder {
				router := gin.New()
				(&HTTP{Store: s, Runner: runner}).Register(router)
				req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/bot-callbacks/slack/%d/%d/command", channel.ID, channel.Revision), strings.NewReader(string(body)))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-Slack-Request-Timestamp", stamp)
				req.Header.Set("X-Slack-Signature", signature)
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				return res
			}
			runner := &Runner{Store: s}
			if change == "valid" {
				if res := httpCall(nil, signature); res.Code != 503 {
					t.Fatal("missing runner", res.Code)
				}
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				unavailable := &Runner{Store: s}
				unavailable.state.Store(&workerRunState{ctx: stopped})
				if res := httpCall(unavailable, signature); res.Code != 503 {
					t.Fatal("stopped runner", res.Code)
				}
				if res := httpCall(runner, "invalid"); res.Code != 403 {
					t.Fatal("forged reaudit", res.Code)
				}
			}
			res := httpCall(runner, signature)
			want := 403
			if change == "valid" {
				want = 200
			}
			if change == "quota" {
				want = 429
			}
			if res.Code != want {
				t.Fatal("HTTP reaudit", change, res.Code, res.Body.String())
			}
			if change == "valid" {
				if !strings.Contains(res.Body.String(), "already queued") || !strings.Contains(res.Body.String(), `"response_type":"ephemeral"`) {
					t.Fatal(res.Body.String())
				}
				if replay := httpCall(runner, signature); replay.Code != 403 {
					t.Fatal("HTTP replay", replay.Code)
				}
			}
		})
	}
}
