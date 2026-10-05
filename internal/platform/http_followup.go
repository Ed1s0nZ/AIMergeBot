package platform

import (
	"errors"
	"github.com/gin-gonic/gin"
)

func (h *HTTP) runScope(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, ok = h.requireRun(c, id, "viewer"); !ok {
		return
	}
	if h.Runner == nil {
		c.JSON(503, gin.H{"error": "audit service unavailable"})
		return
	}
	scope, err := h.Runner.Scope(c.Request.Context(), id, currentUser(c).ID)
	if err != nil {
		if errors.Is(err, ErrFollowupScope) {
			c.JSON(422, gin.H{"error": ErrFollowupScope.Error()})
		} else {
			fail(c, err)
		}
		return
	}
	c.JSON(200, scope)
}
func (h *HTTP) followupRun(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, ok = h.requireRun(c, id, "operator"); !ok {
		return
	}
	var request struct {
		Files []string `json:"files"`
	}
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(400, gin.H{"error": "changed files required"})
		return
	}
	if h.Runner == nil {
		c.JSON(503, gin.H{"error": "audit service unavailable"})
		return
	}
	next, created, err := h.Runner.SubmitFollowup(c.Request.Context(), id, currentUser(c).ID, request.Files)
	if err != nil {
		if errors.Is(err, ErrFollowupScope) {
			c.JSON(422, gin.H{"error": ErrFollowupScope.Error()})
		} else {
			fail(c, err)
		}
		return
	}
	c.JSON(202, gin.H{"id": next, "created": created})
}
