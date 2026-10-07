package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestJiraMappingMetadataAndCredentialPreservation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	team := "10001"
	input := IntegrationInput{Integration: Integration{Name: "Jira", Kind: "jira", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://fixture.atlassian.net", Username: "fixture@example.com", Token: "fixture-private-token", JiraProjectID: team, JiraIssueTypeID: "10002"}}
	integration, err := s.SaveIntegration(ctx, 0, 1, input)
	if err != nil || !integration.HasJiraMapping {
		t.Fatal(integration, err)
	}
	input.Integration = integration
	input.Name = "Renamed"
	input.Credentials = nil
	input.ExpectedRevision = &integration.Revision
	updated, err := s.SaveIntegration(ctx, integration.ID, 1, input)
	if err != nil || !updated.HasJiraMapping {
		t.Fatal("team lost on metadata save", updated, err)
	}
	items, err := s.Integrations(ctx, 1)
	if err != nil || len(items) != 1 || !items[0].HasJiraMapping {
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
	if err := json.Unmarshal([]byte(stored), &credentials); err != nil || credentials.JiraProjectID != team || credentials.JiraIssueTypeID != "10002" || credentials.Token != "fixture-private-token" {
		t.Fatal("stored mapping changed", err)
	}
}

func TestJiraMappingRejectsInvalidIdentifiers(t *testing.T) {
	for _, value := range []string{"0", "-1", "10001\n", "../project", "999999999999999999999"} {
		for _, c := range []IntegrationCredentials{{JiraProjectID: value}, {JiraIssueTypeID: value}} {
			if err := validateIntegrationCredentials("jira", c, false); err == nil {
				t.Fatal("invalid mapping accepted", value)
			}
		}
	}
	// Legacy configurations without a mapping remain readable, but will not
	// become eligible for ticket creation until the mapping is completed.
	if err := validateIntegrationCredentials("jira", IntegrationCredentials{}, false); err != nil {
		t.Fatal(err)
	}
}
