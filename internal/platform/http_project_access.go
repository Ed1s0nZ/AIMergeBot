package platform

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) requireProject(c *gin.Context, project int, required string) (string, bool) {
	role, err := requireProjectRole(c.Request.Context(), h.Store.DB, project, currentUser(c).ID, required)
	if err != nil {
		fail(c, err)
		return "", false
	}
	return role, true
}

func (h *HTTP) requireRun(c *gin.Context, id int64, required string) (string, bool) {
	snap, err := snapshotForRun(c.Request.Context(), h.Store.DB, id)
	if err != nil {
		fail(c, err)
		return "", false
	}
	role, err := requireSnapshotRole(c.Request.Context(), h.Store.DB, snap, currentUser(c).ID, required)
	if err != nil {
		fail(c, err)
		return "", false
	}
	return role, true
}

func (h *HTTP) projectMembers(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := h.Store.ProjectMembers(c.Request.Context(), int(id))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *HTTP) setProjectMember(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	user, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || user <= 0 {
		c.JSON(400, gin.H{"error": "invalid user id"})
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if c.Request.Method != http.MethodDelete {
		if c.ShouldBindJSON(&req) != nil || roleRank(req.Role) == 0 || req.Role == "admin" {
			c.JSON(400, gin.H{"error": "invalid project role"})
			return
		}
	}
	if err = h.Store.SetProjectMember(c.Request.Context(), int(id), user, req.Role, currentUser(c).ID); err != nil {
		fail(c, err)
		return
	}
	if req.Role != "operator" && h.Runner != nil {
		h.Runner.interruptRevoked(int(id), user)
	}
	c.Status(http.StatusNoContent)
}
