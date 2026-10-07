package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJiraTicketPayloadAndAcknowledgement(t *testing.T) {
	title := `Quoted "finding"`
	body, err := buildJiraTicket("10001", "10002", title, "Fixed commit\nEvidence link")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Fields struct {
			Summary     string
			Project     map[string]string
			Issuetype   map[string]string
			Description struct {
				Type    string
				Version int
				Content []json.RawMessage
			}
		}
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Fields.Summary != title || payload.Fields.Project["id"] != "10001" || payload.Fields.Issuetype["id"] != "10002" || payload.Fields.Description.Type != "doc" || payload.Fields.Description.Version != 1 || len(payload.Fields.Description.Content) != 1 {
		t.Fatal(string(body), err)
	}
	for _, bad := range []string{"", "0", "-1", "1\n", "1\"", "999999999999999999999"} {
		if _, err := buildJiraTicket(bad, "10002", title, "text"); err == nil {
			t.Fatal("invalid project accepted", bad)
		}
	}
	endpoint := "https://fixture.atlassian.net"
	good := `{"id":"10003","key":"AUDIT-12","self":"https://fixture.atlassian.net/rest/api/3/issue/10003"}`
	receipt := jiraTicketReceipt(201, []byte(good), endpoint)
	if receipt.State != "created" || receipt.RemoteID != "10003" || receipt.URL != endpoint+"/browse/AUDIT-12" {
		t.Fatal(receipt)
	}
	for _, bad := range []string{"null", "{", strings.ReplaceAll(good, "fixture.atlassian.net", "attacker.example"), strings.ReplaceAll(good, "AUDIT-12", "../private"), strings.TrimSuffix(good, "}") + `,"errors":{"summary":"PRIVATE"}}`, strings.Repeat("x", 65537)} {
		if got := jiraTicketReceipt(201, []byte(bad), endpoint); got.State != "unknown" || got.RemoteID != "" || got.URL != "" {
			t.Fatal("unverified receipt", got)
		}
	}
	for _, status := range []int{200, 400, 401, 429, 500} {
		if got := jiraTicketReceipt(status, []byte(good), endpoint); got.State != "unknown" {
			t.Fatal(status, got)
		}
	}
}
