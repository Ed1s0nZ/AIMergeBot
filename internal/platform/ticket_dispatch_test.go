package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTicketDispatcherPersistsReceiptWithoutReplay(t *testing.T) {
	for _, mode := range []string{"ack", "revoked", "post_changed", "missing_team", "transport_unknown"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
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
			if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"summary":"PRIVATE SOURCE","findings":[{"id":"f","severity":"high"}]}' WHERE id=?`, run); err != nil {
				t.Fatal(err)
			}
			team := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
			credentials := IntegrationCredentials{Endpoint: "https://api.linear.app/graphql", Token: "fixture", LinearTeamID: team}
			zero := int64(0)
			input := IntegrationInput{Integration: Integration{Name: "Linear", Kind: "linear", Enabled: true, ProjectIDs: []int{1}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &credentials}
			integration, err := s.SaveIntegration(ctx, 0, 1, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ReserveFindingTicket(ctx, run, 1, integration.ID, integration.Revision, "f", head); err != nil {
				t.Fatal(err)
			}
			if mode == "missing_team" {
				credentials.LinearTeamID = ""
				raw, _ := json.Marshal(credentials)
				if _, err := s.DB.Exec(`UPDATE platform_integrations SET credentials=? WHERE id=?`, string(raw), integration.ID); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "revoked" {
				if _, err := s.DB.Exec(`UPDATE platform_integrations SET enabled=0 WHERE id=?`, integration.ID); err != nil {
					t.Fatal(err)
				}
			}
			posts := 0
			client := &http.Client{Transport: retryTransportFunc(func(req *http.Request) (*http.Response, error) {
				posts++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(body), "PRIVATE SOURCE") {
					t.Error("raw audit data exported")
				}
				var payload struct {
					Variables struct{ Input map[string]string }
				}
				if err := json.Unmarshal(body, &payload); err != nil || payload.Variables.Input["teamId"] != team || !strings.Contains(payload.Variables.Input["description"], head) {
					t.Error("wrong captured mapping", err)
				}
				if mode == "post_changed" {
					if _, err := s.DB.Exec(`UPDATE platform_integrations SET revision=revision+1 WHERE id=?`, integration.ID); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "transport_unknown" {
					return nil, errors.New("PRIVATE TOKEN")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"issueCreate":{"success":true,"issue":{"id":"` + team + `","url":"https://linear.app/example/issue/LIN-123/finding"}}}}`))}, nil
			})}
			claimed, err := s.DispatchFindingTicket(ctx, "https://audit.example", client)
			if err != nil || !claimed {
				t.Fatal(claimed, err)
			}
			link, err := s.FindingTicket(ctx, run, 1, integration.ID, "f")
			want := map[string]string{"ack": "created", "revoked": "failed", "post_changed": "unknown", "missing_team": "failed", "transport_unknown": "unknown"}[mode]
			if err != nil || link.State != want {
				t.Fatal(link, err, want)
			}
			wantCode := map[string]string{"ack": "", "revoked": "permission_changed", "post_changed": "permission_changed", "missing_team": "invalid_configuration", "transport_unknown": "creation_unacknowledged"}[mode]
			if link.ErrorCode != wantCode || link.UpdatedAt == "" {
				t.Fatal("missing safe diagnosis", link)
			}
			if want == "failed" && posts != 0 || want != "failed" && posts != 1 {
				t.Fatal("wrong POST count", posts)
			}
			if mode == "ack" || mode == "post_changed" {
				if link.RemoteID != team {
					t.Fatal("receipt lost", link)
				}
			}
			if claimed, err := s.DispatchFindingTicket(ctx, "https://audit.example", client); claimed || !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("terminal ticket replayed", claimed, err)
			}
			credentials.LinearTeamID = "invalid-team"
			if err := validateIntegrationCredentials("linear", credentials, true); !errors.Is(err, ErrIntegrationInput) {
				t.Fatal("invalid team accepted", err)
			}
		})
	}
}
