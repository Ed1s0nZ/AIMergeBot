package platform

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) contextRepositories(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := h.Store.ContextRepositories(c.Request.Context(), int(id))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
func (h *HTTP) saveContextRepositories(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req struct {
		Items []ContextRepository `json:"items"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Items == nil {
		c.JSON(400, gin.H{"error": "items must be an array of pinned context repositories"})
		return
	}
	if err := h.Store.SaveContextRepositories(c.Request.Context(), int(id), req.Items, currentUser(c).ID); err != nil {
		if errors.Is(err, ErrContextRepository) {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		fail(c, err)
		return
	}
	if h.Runner != nil {
		h.Runner.interruptContextRevoked(int(id))
	}
	pending := false
	if h.Settings != nil {
		pending = h.Store.SyncProjectConfig(c.Request.Context(), h.Settings) != nil
	}
	c.JSON(200, gin.H{"config_sync_pending": pending})
}
