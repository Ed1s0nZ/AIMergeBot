package platform

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) issueBotBinding(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&req) != nil || req.ExpectedRevision <= 0 {
		c.JSON(400, gin.H{"error": "channel revision required"})
		return
	}
	challenge, err := h.Store.issueSlackBindingChallenge(c.Request.Context(), currentUser(c).ID, id, req.ExpectedRevision, time.Now())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, gin.H{"token": challenge.Token, "expires_at": challenge.ExpiresAt})
}

func (h *HTTP) revokeBotBinding(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	changed, err := h.Store.revokeSlackBinding(c.Request.Context(), currentUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"revoked": changed})
}
