package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestJiraSenderAuthorizationAndSafeOutcomes(t *testing.T) {
	for _, mode := range []string{"ack", "before", "after", "transport", "forged", "configuration"} {
		t.Run(mode, func(t *testing.T) {
			c := IntegrationCredentials{Endpoint: "https://fixture.atlassian.net", Username: "fixture@example.com", Token: "PRIVATE_TOKEN", JiraProjectID: "10001", JiraIssueTypeID: "10002"}
			if mode == "configuration" {
				c.Endpoint += "?token=private"
			}
			calls, checks := 0, 0
			authorize := func(context.Context) error {
				checks++
				if mode == "before" || mode == "after" && checks > 1 {
					return errors.New("PRIVATE_PERMISSION_ERROR")
				}
				return nil
			}
			client := &http.Client{Transport: retryTransportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				user, token, ok := req.BasicAuth()
				if !ok || user != c.Username || token != c.Token || req.URL.String() != c.Endpoint+"/rest/api/3/issue" || req.Method != "POST" {
					t.Fatal("wrong request identity")
				}
				var payload struct {
					Fields struct {
						Project   map[string]string
						Issuetype map[string]string
					}
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Fields.Project["id"] != c.JiraProjectID || payload.Fields.Issuetype["id"] != c.JiraIssueTypeID {
					t.Fatal("wrong configured mapping", err)
				}
				if _, ok := req.Context().Deadline(); !ok {
					t.Error("missing deadline")
				}
				if mode == "transport" {
					return nil, errors.New("PRIVATE_TRANSPORT_ERROR")
				}
				reply := `{"id":"10003","key":"AUDIT-12","self":"https://fixture.atlassian.net/rest/api/3/issue/10003"}`
				if mode == "forged" {
					reply = strings.ReplaceAll(reply, "fixture.atlassian.net", "attacker.example")
				}
				return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
			})}
			got := sendJiraTicket(context.Background(), c, "Finding", "Fixed evidence", authorize, client)
			want := map[string]string{"ack": "created", "before": "failed", "after": "unknown", "transport": "unknown", "forged": "unknown", "configuration": "failed"}[mode]
			if got.State != want {
				t.Fatal(got, want)
			}
			if mode == "before" || mode == "configuration" {
				if calls != 0 {
					t.Fatal("unauthorized POST")
				}
			} else if calls != 1 {
				t.Fatal("unexpected POST count", calls)
			}
			if mode == "after" && (got.RemoteID != "10003" || got.Code != "permission_changed") {
				t.Fatal("ack lost after revocation", got)
			}
			if strings.Contains(got.Code, "PRIVATE") {
				t.Fatal("upstream error leaked")
			}
		})
	}
}
