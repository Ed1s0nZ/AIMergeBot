package platform

import "github.com/gin-gonic/gin"

func (h *HTTP) runStatus(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	role, ok := h.requireRun(c, id, "viewer")
	if !ok {
		return
	}
	status, err := h.detailStatus(c.Request.Context(), id, currentUser(c).ID, role)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, status)
}
