package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLinearTeamMetadataAndCredentialPreservation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	team := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
	input := IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture-private-token", LinearTeamID: team}}
	integration, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil || !integration.HasTeamMapping {
		t.Fatal(integration, err)
	}
	input.Integration = integration
	input.Name = "Renamed"
	input.Credentials = nil
	input.ExpectedRevision = &integration.Revision
	updated, err := s.SaveIntegration(ctx, integration.ID, 1, input)
	if err != nil || !updated.HasTeamMapping {
		t.Fatal("team lost on metadata save", updated, err)
	}
	items, err := s.Integrations(ctx, 1)
	if err != nil || len(items) != 1 || !items[0].HasTeamMapping {
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
	if err := json.Unmarshal([]byte(stored), &credentials); err != nil || credentials.LinearTeamID != team || credentials.Token != "fixture-private-token" {
		t.Fatal("stored mapping changed", err)
	}
}
