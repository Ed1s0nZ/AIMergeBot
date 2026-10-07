package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSlackBindingChallengeHashRotationAndDisabledUser(t *testing.T) {
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
	first, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at)
	if err != nil || len(first.Token) != 48 || !first.ExpiresAt.Equal(at.Add(10*time.Minute)) {
		t.Fatal(first.ExpiresAt, err)
	}
	var stored string
	if err := s.DB.QueryRow(`SELECT token_hash FROM platform_bot_binding_challenges`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(first.Token))
	if stored != hex.EncodeToString(digest[:]) || stored == first.Token {
		t.Fatal("raw binding token stored")
	}
	second, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at)
	if err != nil || second.Token == first.Token {
		t.Fatal("challenge not rotated", err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_bot_binding_challenges`).Scan(&count); err != nil || count != 1 {
		t.Fatal("old challenge retained", count, err)
	}
	if _, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision+1, at); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.issueSlackBindingChallenge(ctx, 1, channel.ID, channel.Revision, at); !errors.Is(err, ErrCredentials) || got.Token != "" {
		t.Fatal("disabled user challenge", err)
	}
}
