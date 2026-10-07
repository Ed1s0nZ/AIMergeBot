package platform

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *HTTP) integrations(c *gin.Context) {
	items, err := h.Store.Integrations(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (h *HTTP) saveIntegration(c *gin.Context) {
	id := int64(0)
	if c.Request.Method == http.MethodPatch {
		var ok bool
		id, ok = idParam(c)
		if !ok {
			return
		}
	}
	var req IntegrationInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32768)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid integration configuration"})
		return
	}
	out, err := h.Store.SaveIntegration(c.Request.Context(), id, currentUser(c).ID, req)
	if errors.Is(err, ErrIntegrationInput) {
		c.JSON(400, gin.H{"error": "invalid integration configuration"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, out)
}
