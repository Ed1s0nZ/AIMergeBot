package platform

import (
	"context"
	"strings"
	"testing"
)

func TestTicketPreflightRechecksForkPolicyAndLease(t *testing.T) {
	for _, mode := range []string{"valid", "source_permission", "source_disabled", "revision", "disabled", "scope", "credentials", "lease", "actor", "head"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
				t.Fatal(err)
			}
			actor, err := s.CreateUser(ctx, "operator", "a-long-password", "member")
			if err != nil {
				t.Fatal(err)
			}
			for _, project := range []int{1, 2} {
				if err := s.SaveProject(ctx, Project{ID: project, Name: "fixture", Enabled: true}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,'operator')`, project, actor.ID); err != nil {
					t.Fatal(err)
				}
			}
			head := strings.Repeat("b", 40)
			run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
				t.Fatal(err)
			}
			zero := int64(0)
			integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ReserveFindingTicket(ctx, run, actor.ID, integration.ID, integration.Revision, "f", head); err != nil {
				t.Fatal(err)
			}
			claim, err := s.ClaimFindingTicket(ctx)
			if err != nil {
				t.Fatal(err)
			}
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := s.DB.Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "source_permission":
				exec(`DELETE FROM platform_project_members WHERE project_id=2 AND user_id=?`, actor.ID)
			case "source_disabled":
				exec(`UPDATE platform_projects SET enabled=0 WHERE id=2`)
			case "revision":
				exec(`UPDATE platform_integrations SET revision=revision+1 WHERE id=?`, integration.ID)
			case "disabled":
				exec(`UPDATE platform_integrations SET enabled=0 WHERE id=?`, integration.ID)
			case "scope":
				exec(`UPDATE platform_integrations SET project_ids='[2]' WHERE id=?`, integration.ID)
			case "credentials":
				exec(`UPDATE platform_integrations SET credentials='invalid' WHERE id=?`, integration.ID)
			case "lease":
				claim.Lease = "old"
			case "actor":
				claim.Actor = 1
			case "head":
				claim.HeadSHA = strings.Repeat("c", 40)
			}
			snap, credentials, err := s.TicketSendContext(ctx, claim)
			if mode == "valid" {
				if err != nil || snap.HeadSHA != head || credentials.Token != "fixture" {
					t.Fatal(snap, credentials, err)
				}
			} else if err == nil || credentials.Token != "" || snap.ProjectID != 0 {
				t.Fatal("invalid preflight returned credentials", mode, err)
			}
		})
	}
}
