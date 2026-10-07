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
}
