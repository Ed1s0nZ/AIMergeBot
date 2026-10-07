package platform

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Slack authenticates this endpoint with its signature, not a browser session.
// Commands grant no project access: binding, status and fixed-commit reaudits
// each perform their own signed transaction and current authorization checks.
func (h *HTTP) slackBindingCallback(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	revision, revisionErr := strconv.ParseInt(c.Param("revision"), 10, 64)
	media, _, mediaErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || revisionErr != nil || id <= 0 || revision <= 0 || mediaErr != nil || media != "application/x-www-form-urlencoded" {
		c.JSON(400, gin.H{"error": "invalid callback"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid callback"})
		return
	}
	if strings.HasSuffix(c.FullPath(), "/command") {
		fields, parseErr := url.ParseQuery(string(body))
		if parseErr == nil && len(fields["text"]) == 1 && strings.HasPrefix(fields.Get("text"), "reaudit ") {
			if h.Runner == nil || h.Runner.workerStopped() {
				c.Header("Retry-After", "30")
				c.JSON(503, gin.H{"error": "audit service unavailable"})
				return
			}
			next, created, err := h.Store.slackReaudit(c.Request.Context(), id, revision, c.GetHeader("X-Slack-Request-Timestamp"), c.GetHeader("X-Slack-Signature"), body, time.Now())
			if err != nil {
				var quota *QuotaError
				if errors.As(err, &quota) {
					c.Header("Retry-After", "60")
					c.JSON(429, gin.H{"error": "reaudit capacity reached"})
				} else {
					c.JSON(403, gin.H{"error": "callback rejected"})
				}
				return
			}
			caption := "already queued"
			if created {
				caption = "queued"
			}
			c.JSON(200, gin.H{"response_type": "ephemeral", "text": fmt.Sprintf("Audit #%d %s for the requested fixed commit.", next, caption), "unfurl_links": false, "unfurl_media": false})
			return
		}
		if parseErr == nil && len(fields["text"]) == 1 && strings.HasPrefix(fields.Get("text"), "status ") {
			out, err := h.Store.slackRunStatus(c.Request.Context(), id, revision, c.GetHeader("X-Slack-Request-Timestamp"), c.GetHeader("X-Slack-Signature"), body, time.Now())
			if err != nil {
				c.JSON(403, gin.H{"error": "callback rejected"})
				return
			}
			text := fmt.Sprintf("Audit #%d · %s · HEAD %s", out.ID, out.Status, out.HeadSHA)
			if h.Settings != nil {
				if link := slackEvidenceLink(h.Settings.Snapshot().PublicURL, out.ID); link != "" {
					text += "\nEvidence (platform login required): " + link
				}
			}
			c.JSON(200, gin.H{"response_type": "ephemeral", "text": text, "unfurl_links": false, "unfurl_media": false})
			return
		}
	}
	if _, err := h.Store.completeSlackBinding(c.Request.Context(), id, revision, c.GetHeader("X-Slack-Request-Timestamp"), c.GetHeader("X-Slack-Signature"), body, time.Now()); err != nil {
		// Do not disclose account, challenge or channel existence to callers.
		c.JSON(403, gin.H{"error": "callback rejected"})
		return
	}
	c.JSON(200, gin.H{"response_type": "ephemeral", "text": "AIMergeBot identity bound. Project permissions remain those of your platform account."})
}
