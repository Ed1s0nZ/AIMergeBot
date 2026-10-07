package platform

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationPersistenceRedactionRevisionAndPermission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integration.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	ctx := context.Background()
	if err = s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	user, err := s.CreateUser(ctx, "reader", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	rev := int64(0)
	input := IntegrationInput{Integration: Integration{Name: "team", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Events: []string{"run.failed"}, Frequency: "instant"}, ExpectedRevision: &rev, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/services/PRIVATE-ENDPOINT", Secret: "PRIVATE-SECRET"}}
	if _, err = s.SaveIntegration(ctx, 0, user.ID, input); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("member allowed", err)
	}
	saved, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil || saved.Revision != 1 || !saved.HasEndpoint || !saved.HasSecret {
		t.Fatal(saved, err)
	}
	items, err := s.Integrations(ctx, 1)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "hooks.slack") {
		t.Fatal("credential leak", string(body))
	}
	if _, err = s.Integrations(ctx, user.ID); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("member read credentials", err)
	}
	input.Credentials = nil
	if _, err = s.SaveIntegration(ctx, saved.ID, 1, input); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision", err)
	}
	rev = saved.Revision
	input.Name = "renamed"
	updated, err := s.SaveIntegration(ctx, saved.ID, 1, input)
	if err != nil || updated.Revision != 2 || !updated.HasSecret {
		t.Fatal(updated, err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_notification_deliveries(integration_id,integration_revision,project_id,event_key,next_attempt,payload,created_at,updated_at) VALUES(?,2,1,'event','', '{}','','')`, saved.ID); err != nil {
		t.Fatal(err)
	}
	rev = updated.Revision
	input.ClearCredentials = true
	input.Enabled = false
	updated, err = s.SaveIntegration(ctx, saved.ID, 1, input)
	if err != nil || updated.HasSecret || updated.HasEndpoint {
		t.Fatal(updated, err)
	}
	var state string
	if err = s.DB.QueryRow(`SELECT status FROM platform_notification_deliveries`).Scan(&state); err != nil || state != "cancelled" {
		t.Fatal(state, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err = s.Integrations(ctx, 1)
	if err != nil || len(items) != 1 || items[0].Revision != 3 || items[0].Enabled {
		t.Fatal(items, err)
	}
}
func TestIntegrationRejectsInvalidRecipientsAndConfiguration(t *testing.T) {
	if err := validateIntegrationCredentials("email", IntegrationCredentials{SMTPHost: "smtp.example", SMTPPort: 465, SMTPMode: "tls", From: "sender@example.com", Recipients: []string{"to@example.com\r\nBcc: victim@example.com"}}, true); err == nil {
		t.Fatal("header injection accepted")
	}
	for _, url := range []string{"http://hooks.example/secret", "https://user:pass@hooks.example", "javascript:alert(1)"} {
		if err := validateIntegrationCredentials("slack", IntegrationCredentials{Endpoint: url}, true); err == nil {
			t.Fatal(url)
		}
	}
	if err := validateIntegrationCredentials("email", IntegrationCredentials{SMTPHost: "smtp.example", SMTPPort: 465, SMTPMode: "tls", From: "sender@example.com", Recipients: []string{"to@example.com"}}, true); err != nil {
		t.Fatal(err)
	}
}
