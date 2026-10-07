package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSlackBindingMetadataAndCredentialPreservation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	team := "T123"
	input := IntegrationInput{Integration: Integration{Name: "Slack", Kind: "slack", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://hooks.slack.com/fixture", Secret: "fixture-private-token", SlackWorkspaceID: team, SlackAppID: "A123"}}
	integration, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil || !integration.HasSlackBinding {
		t.Fatal(integration, err)
	}
	input.Integration = integration
	input.Name = "Renamed"
	input.Credentials = nil
	input.ExpectedRevision = &integration.Revision
	updated, err := s.SaveIntegration(ctx, integration.ID, 1, input)
	if err != nil || !updated.HasSlackBinding {
		t.Fatal("team lost on metadata save", updated, err)
	}
	items, err := s.Integrations(ctx, 1)
	if err != nil || len(items) != 1 || !items[0].HasSlackBinding {
		t.Fatal(items, err)
	}
	body, err := json.Marshal(items)
	if err != nil || strings.Contains(string(body), team) || strings.Contains(string(body), "fixture-private-token") {
		t.Fatal("credential values returned", err)
	}
	var stored string
	if err := s.DB.QueryRow(`SELECT credentials FROM platform_integrations WHERE id=?`, integration.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var credentials IntegrationCredentials
	if err := json.Unmarshal([]byte(stored), &credentials); err != nil || credentials.SlackWorkspaceID != team || credentials.Secret != "fixture-private-token" {
		t.Fatal("stored mapping changed", err)
	}
}
