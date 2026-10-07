package platform

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var jiraNumericID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var jiraIssueKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}-[1-9][0-9]{0,19}$`)

// Jira Cloud v3 uses Atlassian Document Format for description. IDs bind
// the configured project and issue type without relying on mutable names.
func buildJiraTicket(projectID, typeID, title, description string) ([]byte, error) {
	if !jiraNumericID.MatchString(projectID) || !jiraNumericID.MatchString(typeID) || title == "" || title != strings.TrimSpace(title) || len(title) > 255 || strings.ContainsAny(title, "\x00\r\n") || len(description) > 6000 || strings.ContainsRune(description, 0) {
		return nil, ErrIntegrationInput
	}
	return json.Marshal(map[string]any{"fields": map[string]any{"project": map[string]string{"id": projectID}, "issuetype": map[string]string{"id": typeID}, "summary": title, "description": map[string]any{"type": "doc", "version": 1, "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]string{"type": "text", "text": description}}}}}}})
}

func jiraTicketReceipt(status int, body []byte, endpoint string) TicketReceipt {
	unknown := TicketReceipt{State: "unknown", Code: "creation_unacknowledged"}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") || len(endpoint) > 2048 {
		return unknown
	}
	if status != 201 || len(body) > 65536 {
		return unknown
	}
	var result struct {
		ID            string         `json:"id"`
		Key           string         `json:"key"`
		Self          string         `json:"self"`
		Errors        map[string]any `json:"errors"`
		ErrorMessages []string       `json:"errorMessages"`
	}
	if json.Unmarshal(body, &result) != nil || !jiraNumericID.MatchString(result.ID) || !jiraIssueKey.MatchString(result.Key) || len(result.Errors) > 0 || len(result.ErrorMessages) > 0 {
		return unknown
	}
	// The acknowledged resource must belong to the configured Jira instance.
	expected := strings.TrimRight(endpoint, "/") + "/rest/api/3/issue/" + result.ID
	if result.Self != expected {
		return unknown
	}
	return TicketReceipt{State: "created", RemoteID: result.ID, URL: strings.TrimRight(endpoint, "/") + "/browse/" + result.Key}
}
