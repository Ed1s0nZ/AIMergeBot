package platform

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestTicketDatabaseReopenPreservesReceiptAndUnknown(t *testing.T) {
	for _, provider := range []string{"linear", "jira"} {
		t.Run(provider, func(t *testing.T) { testTicketDatabaseReopen(t, provider) })
	}
}
func testTicketDatabaseReopen(t *testing.T, provider string) {
	for _, state := range []string{"created", "unknown", "sending"} {
		t.Run(state, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tickets.db")
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
			head := strings.Repeat("b", 40)
			run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, HeadSHA: head, BaseSHA: strings.Repeat("a", 40)}, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE platform_runs SET status='succeeded',result_json='{"findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
				t.Fatal(err)
			}
			team := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
			credentials := IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: team}
			if provider == "jira" {
				team = "10003"
				credentials = IntegrationCredentials{Endpoint: "https://fixture.atlassian.net", Username: "fixture@example.com", Token: "fixture", JiraProjectID: "10001", JiraIssueTypeID: "10002"}
			}
			zero := int64(0)
			integration, err := s.SaveIntegration(ctx, 0, 1, IntegrationInput{Integration: Integration{Name: provider, Kind: provider, Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &credentials})
			if err != nil {
				t.Fatal(err)
			}
			initial, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := s.ClaimFindingTicket(ctx)
			if err != nil {
				t.Fatal(err)
			}
			remoteURL := "https://linear.app/example/issue/LIN-123/finding"
			if provider == "jira" {
				remoteURL = credentials.Endpoint + "/browse/AUDIT-12"
			}
			if state != "sending" {
				receipt := TicketReceipt{State: state, RemoteID: team, URL: remoteURL}
				if state == "unknown" {
					receipt.Code = "permission_changed"
				}
				if err := s.FinishFindingTicket(ctx, claim, receipt); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.DB.Exec(`UPDATE platform_ticket_links SET lease_until='2000-01-01T00:00:00Z' WHERE id=?`, claim.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			// Recovery commits even when there is no pending work. No remote transport
			// is needed because persisted terminal or expired sends cannot be claimed.
			if _, err := s.ClaimFindingTicket(ctx); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("reopened ticket replayed", err)
			}
			link, err := s.FindingTicket(ctx, run, 1, integration.ID, "f")
			if err != nil {
				t.Fatal(err)
			}
			want := state
			if state == "sending" {
				want = "unknown"
			}
			if link.State != want || link.IdempotencyKey != initial.IdempotencyKey || link.ID != initial.ID || link.EndpointOrigin != credentials.Endpoint {
				t.Fatal("identity or state lost", link)
			}
			if state != "sending" && (link.RemoteID != team || link.URL != remoteURL) {
				t.Fatal("receipt lost", link)
			}
			if state == "sending" && link.ErrorCode != "creation_unacknowledged" {
				t.Fatal("expired send recovery missing", link)
			}
			repeated, created, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head)
			if err != nil || created || repeated.ID != initial.ID || repeated.State != want {
				t.Fatal("reopen reservation duplicated", repeated, created, err)
			}
			if err := s.FinishFindingTicket(ctx, claim, TicketReceipt{State: "failed"}); !errors.Is(err, ErrConflict) {
				t.Fatal("old acknowledgement accepted", err)
			}
		})
	}
}
