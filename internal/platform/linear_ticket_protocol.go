package platform

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var ticketUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type TicketReceipt struct {
	State    string `json:"state"`
	Code     string `json:"code,omitempty"`
	RemoteID string `json:"remote_id,omitempty"`
	URL      string `json:"url,omitempty"`
}

func buildLinearTicket(team, title, description string) ([]byte, error) {
	if !ticketUUID.MatchString(team) || strings.TrimSpace(title) == "" || len(title) > 255 || len(description) > 6000 || strings.ContainsAny(title, "\x00\r\n") || strings.ContainsRune(description, '\x00') {
		return nil, ErrIntegrationInput
	}
	return json.Marshal(map[string]any{
		"query":     `mutation AIMergeBotIssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id url } } }`,
		"variables": map[string]any{"input": map[string]string{"teamId": team, "title": title, "description": description}},
	})
}

func linearTicketReceipt(status int, body []byte) TicketReceipt {
	unknown := TicketReceipt{State: "unknown", Code: "creation_unacknowledged"}
	if status != 200 || len(body) > 65536 {
		return unknown
	}
	var reply struct {
		Errors json.RawMessage `json:"errors"`
		Data   struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return unknown
	}
	if len(reply.Errors) > 0 && string(reply.Errors) != "null" {
		var errs []json.RawMessage
		if json.Unmarshal(reply.Errors, &errs) != nil || len(errs) != 0 {
			return unknown
		}
	}
	created := reply.Data.IssueCreate
	u, err := url.Parse(created.Issue.URL)
	if !created.Success || !ticketUUID.MatchString(created.Issue.ID) || err != nil || len(created.Issue.URL) > 2048 || u.Scheme != "https" || u.Host != "linear.app" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.Contains(u.Path, "/issue/") {
		return unknown
	}
	return TicketReceipt{State: "created", RemoteID: created.Issue.ID, URL: created.Issue.URL}
}
