package platform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

func sendJiraTicket(ctx context.Context, c IntegrationCredentials, title, description string, authorize func(context.Context) error, client *http.Client) TicketReceipt {
	failed := TicketReceipt{State: "failed", Code: "invalid_configuration"}
	if !jiraTicketReady(c) {
		return failed
	}
	body, err := buildJiraTicket(c.JiraProjectID, c.JiraIssueTypeID, title, description)
	if err != nil {
		return failed
	}
	if authorize == nil {
		return TicketReceipt{State: "failed", Code: "permission_changed"}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := authorize(ctx); err != nil {
		return TicketReceipt{State: "failed", Code: "permission_changed"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Endpoint, "/")+"/rest/api/3/issue", bytes.NewReader(body))
	if err != nil {
		return failed
	}
	req.SetBasicAuth(c.Username, c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = notificationHTTPClient(c.AllowedNetworks)
	}
	unknown := TicketReceipt{State: "unknown", Code: "creation_unacknowledged"}
	res, err := client.Do(req)
	if err != nil {
		return unknown
	}
	defer res.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return unknown
	}
	receipt := jiraTicketReceipt(res.StatusCode, reply, c.Endpoint)
	if receipt.State == "created" {
		if err := authorize(ctx); err != nil {
			receipt.State = "unknown"
			receipt.Code = "permission_changed"
		}
	}
	return receipt
}
