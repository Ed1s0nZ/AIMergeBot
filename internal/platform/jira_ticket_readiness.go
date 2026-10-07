package platform

import (
	"net/url"
	"strings"
)

func jiraTicketReady(c IntegrationCredentials) bool {
	u, err := url.Parse(c.Endpoint)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && len(c.Endpoint) <= 2048 && jiraNumericID.MatchString(c.JiraProjectID) && jiraNumericID.MatchString(c.JiraIssueTypeID) && c.Username != "" && !strings.ContainsAny(c.Username, ":\r\n") && c.Username == strings.TrimSpace(c.Username) && c.Token != "" && !strings.ContainsAny(c.Token, "\r\n") && c.Token == strings.TrimSpace(c.Token) && validateIntegrationCredentials("jira", c, true) == nil
}
