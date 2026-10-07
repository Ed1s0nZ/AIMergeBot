package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFindingTicketChannelsScopeReadinessAndSecrets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser(ctx, "viewer", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, HeadSHA: strings.Repeat("b", 40), BaseSHA: strings.Repeat("a", 40)}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	credentials := IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "PRIVATE_TOKEN", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear fixture", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &credentials})
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.FindingTicketChannels(ctx, run, 1, "f")
	if err != nil || len(items) != 1 || items[0].ID != integration.ID || items[0].Revision != integration.Revision {
		t.Fatal(items, err)
	}
	encoded, _ := json.Marshal(items)
	if strings.Contains(string(encoded), credentials.Token) || strings.Contains(string(encoded), credentials.LinearTeamID) || strings.Contains(string(encoded), "credentials") {
		t.Fatal("credentials disclosed")
	}
	if _, err := s.FindingTicketChannels(ctx, run, viewer.ID, "f"); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer channel access", err)
	}
	if _, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", strings.Repeat("b", 40)); err != nil {
		t.Fatal(err)
	}
	links, err := s.FindingTickets(ctx, run, viewer.ID, "f")
	if err != nil || len(links) != 1 || links[0].State != "pending" {
		t.Fatal(links, err)
	}
	public, _ := json.Marshal(links)
	if strings.Contains(string(public), "idempotency_key") {
		t.Fatal("internal key disclosed")
	}
	if _, err := s.FindingTickets(ctx, run, viewer.ID, "missing"); err == nil {
		t.Fatal("missing finding list accepted")
	}
	for _, mutation := range []string{`enabled=0`, `project_ids='[2]'`, `credentials='{}'`, `credentials='broken'`, `kind='jira'`} {
		t.Run(mutation, func(t *testing.T) {
			raw, _ := json.Marshal(credentials)
			if _, err := s.DB.Exec(`UPDATE platform_integrations SET enabled=1,project_ids='[1]',kind='linear',credentials=? WHERE id=?`, string(raw), integration.ID); err != nil {
				t.Fatal(err)
			}
			// Fixed, test-owned mutation strings only.
			if _, err := s.DB.Exec(`UPDATE platform_integrations SET `+mutation+` WHERE id=?`, integration.ID); err != nil {
				t.Fatal(err)
			}
			items, err := s.FindingTicketChannels(ctx, run, 1, "f")
			if err != nil || len(items) != 0 {
				t.Fatal("unusable channel offered", items, err)
			}
		})
	}
	if _, err := s.FindingTicketChannels(ctx, run, 1, "missing"); err == nil {
		t.Fatal("missing finding accepted")
	}
}
