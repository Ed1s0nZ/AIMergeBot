package platform

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

func sendLinearTicket(ctx context.Context, credentials IntegrationCredentials, team, title, description string, authorize func(context.Context) error, client *http.Client) TicketReceipt {
	failed := TicketReceipt{State: "failed", Code: "invalid_configuration"}
	if credentials.Endpoint != "https://api.linear.app/graphql" || credentials.Token == "" || credentials.Token != strings.TrimSpace(credentials.Token) || strings.ContainsAny(credentials.Token, "\r\n") || validateIntegrationCredentials("linear", credentials, true) != nil {
		return failed
	}
	body, err := buildLinearTicket(team, title, description)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.Endpoint, bytes.NewReader(body))
	if err != nil {
		return failed
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", credentials.Token)
	if client == nil {
		client = notificationHTTPClient(credentials.AllowedNetworks)
	}
	res, err := client.Do(req)
	unknown := TicketReceipt{State: "unknown", Code: "creation_unacknowledged"}
	if err != nil {
		return unknown
	}
	defer res.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return unknown
	}
	result := linearTicketReceipt(res.StatusCode, reply)
	if result.State == "created" {
		if err := authorize(ctx); err != nil {
			result.State, result.Code = "unknown", "permission_changed"
		}
	}
	return result
}
