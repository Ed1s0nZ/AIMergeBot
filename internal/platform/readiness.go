package platform

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Readiness is an instantaneous observation. Admission and writes still require
// their own transaction fences; this check cannot grant lasting ownership.
func (h *HTTP) ready(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	unavailable := func() { c.JSON(503, gin.H{"status": "unavailable"}) }
	if h.Store == nil || h.Store.DB == nil || h.Runner == nil {
		unavailable()
		return
	}
	state := h.Runner.state.Load()
	if state == nil || state.owner == "" || state.ctx == nil || state.ctx.Err() != nil {
		unavailable()
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	var live bool
	err := h.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_worker_instance WHERE id=1 AND owner=? AND julianday(lease_until)>julianday('now'))`, state.owner).Scan(&live)
	if err != nil || !live || state.ctx.Err() != nil {
		unavailable()
		return
	}
	c.JSON(200, gin.H{"status": "ready"})
}
