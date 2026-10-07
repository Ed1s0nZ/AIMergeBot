package platform

import "testing"

func TestJiraTicketReadinessRequiresCompleteMapping(t *testing.T) {
	valid := IntegrationCredentials{Endpoint: "https://fixture.atlassian.net", Username: "fixture@example.com", Token: "fixture", JiraProjectID: "10001", JiraIssueTypeID: "10002"}
	if !jiraTicketReady(valid) {
		t.Fatal("valid mapping rejected")
	}
	for _, mutate := range []func(*IntegrationCredentials){
		func(c *IntegrationCredentials) { c.JiraProjectID = "" },
		func(c *IntegrationCredentials) { c.JiraIssueTypeID = "" },
		func(c *IntegrationCredentials) { c.Username = "" },
		func(c *IntegrationCredentials) { c.Username = "user:password" },
		func(c *IntegrationCredentials) { c.Token = "" },
		func(c *IntegrationCredentials) { c.Token = "fixture\r\n" },
		func(c *IntegrationCredentials) { c.Endpoint = "http://fixture.atlassian.net" },
		func(c *IntegrationCredentials) { c.Endpoint += "/private" },
		func(c *IntegrationCredentials) { c.Endpoint += "?token=private" },
	} {
		c := valid
		mutate(&c)
		if jiraTicketReady(c) {
			t.Fatal("invalid mapping accepted")
		}
	}
}
