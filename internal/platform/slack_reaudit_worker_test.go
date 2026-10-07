package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type botSnapshotAuditor struct{ snapshots chan Snapshot }

func (a botSnapshotAuditor) Audit(_ context.Context, snap Snapshot, scope DiffScope) (AuditResult, []ToolTrace, error) {
	a.snapshots <- snap
	return AuditResult{Findings: []Finding{}, Summary: "controlled bot audit", CoverageNotes: scope.Notes}, nil, nil
}

func TestSlackReauditHTTPFirstAdmissionExecutesPinnedSnapshot(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(fmt.Sprint(revoke), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "bot-worker.db")
			s, err := OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
				t.Fatal(err)
			}
			user, err := s.CreateUser(ctx, "operator", "a-long-password", "member")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.SetProjectMember(ctx, 1, user.ID, "operator", 1); err != nil {
				t.Fatal(err)
			}
			snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, DiffVersionID: 7, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: &AuditPolicy{Model: "frozen-model", Excluded: []string{"md"}}}
			parent := lifecycleRun(t, s, snap, nil, "failed")
			zero := int64(0)
			secret := "fixture-secret"
			channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: secret, SlackAppID: "A1", SlackWorkspaceID: "T1"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`INSERT INTO platform_bot_bindings(integration_id,workspace_id,external_user_id,user_id,created_at) VALUES(?,'T1','U1',?,?)`, channel.ID, user.ID, now()); err != nil {
				t.Fatal(err)
			}
			capture := botSnapshotAuditor{snapshots: make(chan Snapshot, 2)}
			runner := &Runner{Store: s, Repository: followupRepo{expected: snap, changes: []Change{{NewPath: "a.any", Diff: "@@ -0,0 +1 @@\n+controlled"}}}, Auditor: capture, Workers: 1, Timeout: 5 * time.Second}
			router := gin.New()
			(&HTTP{Store: s, Runner: runner}).Register(router)
			body := fmt.Sprintf("api_app_id=A1&team_id=T1&user_id=U1&text=reaudit+%d+%s", parent.ID, snap.HeadSHA)
			stamp := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte("v0:" + stamp + ":" + body))
			signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
			request := func() *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/bot-callbacks/slack/%d/%d/command", channel.ID, channel.Revision), strings.NewReader(body))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-Slack-Request-Timestamp", stamp)
				req.Header.Set("X-Slack-Signature", signature)
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				return res
			}
			res := request()
			if res.Code != 200 || !strings.Contains(res.Body.String(), " queued ") {
				t.Fatal(res.Code, res.Body.String())
			}
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			var id int64
			if _, err := fmt.Sscanf(payload.Text, "Audit #%d queued", &id); err != nil || id <= 0 || id == parent.ID {
				t.Fatal(payload, err)
			}
			if revoke {
				if err := s.SetProjectMember(ctx, 1, user.ID, "viewer", 1); err != nil {
					t.Fatal(err)
				}
			}
			if err := runner.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer runner.Stop()
			if revoke {
				waitStatus(t, s, id, "cancelled", 15*time.Second)
				select {
				case <-capture.snapshots:
					t.Fatal("revoked actor executed")
				default:
				}
			} else {
				select {
				case got := <-capture.snapshots:
					if got.HeadSHA != snap.HeadSHA || got.BaseSHA != snap.BaseSHA || got.DiffVersionID != 7 || got.AuditPolicy.Model != "frozen-model" {
						t.Fatal("snapshot switched", got)
					}
				case <-time.After(15 * time.Second):
					t.Fatal("worker did not execute")
				}
				waitStatus(t, s, id, "succeeded", 15*time.Second)
			}
			runner.Stop()
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			runner.Store = s
			router = gin.New()
			(&HTTP{Store: s, Runner: runner}).Register(router)
			if err := runner.Start(ctx); err != nil {
				t.Fatal(err)
			}
			if replay := request(); replay.Code != 403 {
				t.Fatal("replay after worker restart", replay.Code)
			}
			var count int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 2 {
				t.Fatal("replay created run", count, err)
			}
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_bindings WHERE user_id=?`, user.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal("binding lost across database reopen", count, err)
			}
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_callback_receipts`).Scan(&count); err != nil || count != 1 {
				t.Fatal("receipt lost across database reopen", count, err)
			}
		})
	}
}
