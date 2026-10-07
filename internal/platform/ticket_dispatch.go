package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (s *Store) DispatchFindingTicket(ctx context.Context, publicURL string, client *http.Client) (bool, error) {
	claim, err := s.ClaimFindingTicket(ctx)
	if err != nil {
		return false, err
	}
	finish := func(receipt TicketReceipt) (bool, error) { return true, s.FinishFindingTicket(ctx, claim, receipt) }
	snap, credentials, err := s.TicketSendContext(ctx, claim)
	if err != nil {
		return finish(TicketReceipt{State: "failed", Code: "permission_changed"})
	}
	if claim.Provider != "linear" && claim.Provider != "jira" {
		return finish(TicketReceipt{State: "failed", Code: "unsupported_provider"})
	}
	description := fmt.Sprintf("AIMergeBot run #%d\nFinding ID: %s\nCommit: %s", claim.RunID, claim.FindingID, snap.HeadSHA)
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || len(publicURL) > 2048 {
			return finish(TicketReceipt{State: "failed", Code: "invalid_configuration"})
		}
		description += fmt.Sprintf("\n%s/#/runs/%d", strings.TrimRight(publicURL, "/"), claim.RunID)
	}
	authorize := func(ctx context.Context) error { _, _, err := s.TicketSendContext(ctx, claim); return err }
	var receipt TicketReceipt
	title := fmt.Sprintf("AIMergeBot finding in run #%d", claim.RunID)
	if claim.Provider == "jira" {
		receipt = sendJiraTicket(ctx, credentials, title, description, authorize, client)
	} else {
		receipt = sendLinearTicket(ctx, credentials, credentials.LinearTeamID, title, description, authorize, client)
	}
	return finish(receipt)
}
