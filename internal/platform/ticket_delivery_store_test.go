package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestTicketClaimExpiryAndReceiptFencing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"},{"id":"g","severity":"high"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"lease", "lease_until", "error_code"} {
		if _, err := s.DB.Exec(`ALTER TABLE platform_ticket_links DROP COLUMN ` + column); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimFindingTicket(ctx)
	if err != nil || claim.ID != first.ID || claim.IdempotencyKey != first.IdempotencyKey || len(claim.Lease) != 48 {
		t.Fatal(claim, err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_ticket_links SET lease_until='2000-01-01T00:00:00Z' WHERE id=?`, claim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimFindingTicket(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("expired job retried", err)
	}
	link, err := s.FindingTicket(ctx, run, 1, integration.ID, "f")
	if err != nil || link.State != "unknown" {
		t.Fatal("expiry rolled back", link, err)
	}
	if err := s.FinishFindingTicket(ctx, claim, TicketReceipt{State: "failed"}); !errors.Is(err, ErrConflict) {
		t.Fatal("expired acknowledgement accepted", err)
	}
	if _, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "g", head); err != nil {
		t.Fatal(err)
	}
	claim, err = s.ClaimFindingTicket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := TicketReceipt{State: "created", RemoteID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d", URL: "https://linear.app/example/issue/LIN-123/finding"}
	for _, field := range []string{"lease", "head", "actor", "key", "revision", "origin"} {
		forged := claim
		switch field {
		case "lease":
			forged.Lease = "old"
		case "head":
			forged.HeadSHA = strings.Repeat("c", 40)
		case "actor":
			forged.Actor++
		case "key":
			forged.IdempotencyKey = "other"
		case "origin":
			forged.EndpointOrigin = "https://attacker.example"
		case "revision":
			forged.IntegrationRevision++
		}
		if err := s.FinishFindingTicket(ctx, forged, receipt); !errors.Is(err, ErrConflict) {
			t.Fatal("forged receipt accepted", field, err)
		}
	}
	if err := s.FinishFindingTicket(ctx, claim, TicketReceipt{State: "created"}); !errors.Is(err, ErrConflict) {
		t.Fatal("missing receipt accepted", err)
	}
	if err := s.FinishFindingTicket(ctx, claim, receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishFindingTicket(ctx, claim, receipt); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate acknowledgement accepted", err)
	}
	if _, err := s.ClaimFindingTicket(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("terminal jobs retried", err)
	}
}

func TestJiraReceiptFencedToPersistedOrigin(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"},{"id":"g","severity":"high"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: "9cfb482a-81e3-4154-b5b9-2c805e70a02d"}})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
	if err != nil {
		t.Fatal(err)
	}

	const endpoint = "https://fixture.atlassian.net"
	if _, err := s.DB.Exec(`UPDATE platform_ticket_links SET provider='jira',endpoint_origin=? WHERE id=?`, endpoint, first.ID); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimFindingTicket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := TicketReceipt{State: "created", RemoteID: "10003", URL: endpoint + "/browse/AUDIT-12"}
	bad := receipt
	bad.URL = "https://attacker.example/browse/AUDIT-12"
	if err := s.FinishFindingTicket(ctx, claim, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign receipt accepted", err)
	}
	forged := claim
	forged.EndpointOrigin = "https://attacker.example"
	if err := s.FinishFindingTicket(ctx, forged, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("forged origin accepted", err)
	}
	if err := s.FinishFindingTicket(ctx, claim, receipt); err != nil {
		t.Fatal(err)
	}
	link, err := s.FindingTicket(ctx, run, 1, integration.ID, "f")
	if err != nil || link.State != "created" || link.URL != receipt.URL || link.EndpointOrigin != endpoint {
		t.Fatal(link, err)
	}
}
