package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLinearTicketVariablesAndAcknowledgement(t *testing.T) {
	id := "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
	title := `Finding "quoted" { mutation }`
	body, err := buildLinearTicket(id, title, "Evidence link only")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Query     string
		Variables struct{ Input map[string]string }
	}
	if err := json.Unmarshal(body, &request); err != nil || strings.Contains(request.Query, title) || request.Variables.Input["title"] != title || request.Variables.Input["teamId"] != id {
		t.Fatal("unsafe mutation interpolation", err)
	}
	for _, team := range []string{"", "team-name", id + "\n"} {
		if _, err := buildLinearTicket(team, title, ""); err == nil {
			t.Fatal("invalid team accepted")
		}
	}
	valid := `{"data":{"issueCreate":{"success":true,"issue":{"id":"` + id + `","url":"https://linear.app/example/issue/LIN-123/finding"}}}}`
	if r := linearTicketReceipt(200, []byte(valid)); r.State != "created" || r.RemoteID != id {
		t.Fatal(r)
	}
	for _, body := range []string{
		`{}`, `null`, `invalid`, strings.Repeat("x", 65537),
		strings.Replace(valid, `"success":true`, `"success":false`, 1),
		strings.Replace(valid, `"data":`, `"errors":[{"message":"PRIVATE TOKEN"}],"data":`, 1),
		strings.Replace(valid, "https://linear.app/", "https://linear.app.attacker.test/", 1),
		strings.Replace(valid, "https://linear.app/", "https://user:password@linear.app/", 1),
	} {
		r := linearTicketReceipt(200, []byte(body))
		if r.State != "unknown" || r.RemoteID != "" || r.URL != "" || strings.Contains(r.Code, "PRIVATE") {
			t.Fatal("unproven or private receipt", r)
		}
	}
	if r := linearTicketReceipt(500, []byte(valid)); r.State != "unknown" {
		t.Fatal(r)
	}
}
