package platform

import (
	"net/url"
	"strconv"
	"strings"
)

// Use the configured platform origin only, never a callback response_url or a
// repository-provided URL. The destination still requires platform login/ACLs.
func slackEvidenceLink(base string, runID int64) string {
	if runID <= 0 || len(base) > 2048 || strings.ContainsAny(base, "<>|\r\n\t ") {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return strings.TrimRight(base, "/") + "/#/runs/" + strconv.FormatInt(runID, 10)
}
