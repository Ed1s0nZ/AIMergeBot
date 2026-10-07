package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTicketProjectsEnabledChecksWholeSnapshot(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, id := range []int{1, 2, 3} {
		if err := s.SaveProject(ctx, Project{ID: id, Name: "fixture", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 2, AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 3, SHA: strings.Repeat("c", 40)}}}}
	for _, disabled := range []int{0, 1, 2, 3} {
		if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=(id!=?)`, disabled); err != nil {
			t.Fatal(err)
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = requireTicketProjectsEnabled(ctx, tx, snap)
		tx.Rollback()
		if disabled == 0 && err != nil || disabled != 0 && !errors.Is(err, ErrProjectPermission) {
			t.Fatal(disabled, err)
		}
	}
}

func TestTicketReservationRejectsDisabledForkBeforeQueue(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err := s.SaveProject(ctx, Project{ID: id, Name: "fixture", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, HeadSHA: head, BaseSHA: strings.Repeat("a", 40)}, 1, false)
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
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head); created || !errors.Is(err, ErrProjectPermission) {
		t.Fatal(created, err)
	}
	if _, err := s.FindingTicketChannels(ctx, run, 1, "f"); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("disabled fork options", err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_ticket_links`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid reservation queued", count, err)
	}
}
