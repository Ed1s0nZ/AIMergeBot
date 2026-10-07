package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSlackBindingCompletionConsumesChallengeAndRejectsReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "callbacks.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	secret := "fixture-signing-secret"
	zero := int64(0)
	channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: secret, SlackAppID: "A1", SlackWorkspaceID: "T1"}})
	if err != nil {
		t.Fatal(err)
	}

	at := time.Unix(1800000000, 0)
	challenge, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at)
	if err != nil {
		t.Fatal(err)
	}
	stamp := strconv.FormatInt(at.Unix(), 10)
	sign := func(body []byte) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte("v0:" + stamp + ":"))
		m.Write(body)
		return "v0=" + hex.EncodeToString(m.Sum(nil))
	}
	body := []byte("api_app_id=A1&team_id=T1&user_id=U1&text=bind+" + challenge.Token)
	if _, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, "v0="+string(make([]byte, 64)), body, at); err == nil {
		t.Fatal("forged binding accepted")
	}
	actor, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, sign(body), body, at)
	if err != nil || actor != 1 {
		t.Fatal(actor, err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_binding_challenges`).Scan(&count); err != nil || count != 0 {
		t.Fatal("challenge not consumed", count, err)
	}
	if _, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, sign(body), body, at); err == nil {
		t.Fatal("binding replay accepted")
	}
	replacement, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at)
	if err != nil {
		t.Fatal(err)
	}
	other := []byte("api_app_id=A1&team_id=T1&user_id=U2&text=bind+" + replacement.Token)
	if _, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, sign(other), other, at); err == nil {
		t.Fatal("existing identity overwritten")
	}
	var external string
	if err := s.DB.QueryRow(`SELECT external_user_id FROM platform_bot_bindings WHERE user_id=1`).Scan(&external); err != nil || external != "U1" {
		t.Fatal("binding changed", external, err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	other = []byte("api_app_id=A1&team_id=T2&user_id=U1&text=bind+" + replacement.Token)
	if _, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, sign(other), other, at); err == nil {
		t.Fatal("disabled account bound")
	}
	if _, err := s.DB.Exec(`UPDATE platform_users SET disabled=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	otherUser, err := s.CreateUser(ctx, "other", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if removed, err := s.revokeSlackBinding(ctx, otherUser.ID, channel.ID); err != nil || removed {
		t.Fatal("another user revoked binding", removed, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_bindings`).Scan(&count); err != nil || count != 1 {
		t.Fatal("owner binding lost", count, err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_integrations SET enabled=0 WHERE id=?`, channel.ID); err != nil {
		t.Fatal(err)
	}
	if removed, err := s.revokeSlackBinding(ctx, 1, channel.ID); err != nil || !removed {
		t.Fatal("owner cannot revoke disabled channel", removed, err)
	}
	for _, table := range []string{"platform_bot_bindings", "platform_bot_binding_challenges"} {
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("revocation retained identity or challenge", table, count, err)
		}
	}
	if removed, err := s.revokeSlackBinding(ctx, 1, channel.ID); err != nil || removed {
		t.Fatal("revocation not idempotent", removed, err)
	}
	for action, expected := range map[string]int{"bot.binding.challenge_issued": 2, "bot.binding.created": 1, "bot.binding.revoked": 1} {
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events WHERE action=? AND actor=1 AND target=?`, action, channel.ID).Scan(&count); err != nil || count != expected {
			t.Fatal("binding audit history mismatch", action, count, err)
		}
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events WHERE target IN (?,?)`, challenge.Token, replacement.Token).Scan(&count); err != nil || count != 0 {
		t.Fatal("challenge leaked into audit history", count, err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_integrations SET enabled=1 WHERE id=?`, channel.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fixture_reject_binding_history BEFORE INSERT ON platform_events WHEN NEW.action='bot.binding.revoked' BEGIN SELECT RAISE(ABORT,'fixture history failure'); END`); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.revokeSlackBinding(ctx, 1, channel.ID); err == nil || changed {
		t.Fatal("history failure accepted", changed, err)
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_binding_challenges WHERE user_id=1`).Scan(&count); err != nil || count != 1 {
		t.Fatal("revocation committed without history", count, err)
	}

}

func TestSlackBindingCompletionRechecksProjectAccess(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	user, err := s.CreateUser(ctx, "member", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, user.ID); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	secret := "fixture-secret"
	channel, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: secret, SlackAppID: "A1", SlackWorkspaceID: "T1"}})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1800000000, 0)
	challenge, err := s.issueSlackBindingChallenge(ctx, user.ID, channel.ID, channel.Revision, at)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("api_app_id=A1&team_id=T1&user_id=U1&text=bind+" + challenge.Token)
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + stamp + ":"))
	mac.Write(body)
	signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE user_id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if actor, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, signature, body, at); err == nil || actor != 0 {
		t.Fatal("revoked project access bound", actor, err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_bindings`).Scan(&count); err != nil || count != 0 {
		t.Fatal("binding written after revoked access", count, err)
	}
	if _, err := s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, signature, body, at); err == nil {
		t.Fatal("disabled project bound")
	}
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if actor, err := s.completeSlackBinding(ctx, channel.ID, channel.Revision, stamp, signature, body, at); err != nil || actor != user.ID {
		t.Fatal("valid callback after failed transaction", actor, err)
	}
}
