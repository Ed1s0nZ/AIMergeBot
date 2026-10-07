package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestTicketReservationPermissionAndDurableIdentity(t *testing.T) {
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
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveFindingTicket(ctx, run, viewer.ID, integration.ID, integration.Revision, "f", head); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer created ticket", err)
	}
	first, created, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
	if err != nil || !created || first.State != "pending" || len(first.IdempotencyKey) != 48 {
		t.Fatal(first, created, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_ticket_links SET state='unknown' WHERE id=?`, first.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	repeated, created, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
	if err != nil || created || repeated.ID != first.ID || repeated.IdempotencyKey != first.IdempotencyKey || repeated.State != "unknown" {
		t.Fatal("unknown creation reset", repeated, created, err)
	}
	if _, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision+1, "f", head); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision accepted", err)
	}
	if _, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", strings.Repeat("c", 40)); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong HEAD accepted", err)
	}
	if _, err := s.FindingTicket(ctx, run, viewer.ID, integration.ID, "f"); err != nil {
		t.Fatal("viewer cannot read", err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE user_id=?`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindingTicket(ctx, run, viewer.ID, integration.ID, "f"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("revoked viewer read", err)
	}
}
