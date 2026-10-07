package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSlackCallbackPersistentAtomicReplayGate(t *testing.T) {
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
	stamp := strconv.FormatInt(at.Unix(), 10)
	body := []byte("api_app_id=A1&team_id=T1&user_id=U1&text=run+1")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + stamp + ":"))
	mac.Write(body)
	signature := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signature, []byte("user_id=U2"), at); !errors.Is(err, ErrConflict) {
		t.Fatal("forged body accepted", err)
	}
	for _, foreign := range []string{strings.Replace(string(body), "api_app_id=A1", "api_app_id=A2", 1), strings.Replace(string(body), "team_id=T1", "team_id=T2", 1), string(body) + "&team_id=T1"} {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte("v0:" + stamp + ":" + foreign))
		signed := "v0=" + hex.EncodeToString(m.Sum(nil))
		if err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signed, []byte(foreign), at); !errors.Is(err, ErrConflict) {
			t.Fatal("foreign or duplicate domain accepted", err)
		}
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signature, body, at)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("replay accepted", accepted.Load())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signature, body, at); !errors.Is(err, ErrConflict) {
		t.Fatal("reopened replay accepted", err)
	}
	var hash string
	if err := s.DB.QueryRow(`SELECT request_hash FROM platform_bot_callback_receipts`).Scan(&hash); err != nil || len(hash) != 64 || strings.Contains(hash, "U1") {
		t.Fatal("raw callback stored", hash, err)
	}
	if err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signature, body, at.Add(301*time.Second)); !errors.Is(err, ErrConflict) {
		t.Fatal("expired callback accepted", err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_integrations SET enabled=0 WHERE id=?`, channel.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.consumeSlackCallback(ctx, channel.ID, channel.Revision, stamp, signature, body, at); err == nil {
		t.Fatal("disabled callback accepted")
	}
}
