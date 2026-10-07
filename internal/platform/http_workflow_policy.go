package platform

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *HTTP) workflowPolicy(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, ok = h.requireProject(c, int(id), "admin"); !ok {
		return
	}
	var exists int
	if err := h.Store.DB.QueryRowContext(c.Request.Context(), `SELECT id FROM platform_projects WHERE id=?`, id).Scan(&exists); err != nil {
		fail(c, err)
		return
	}
	p, err := readWorkflowPolicy(c.Request.Context(), h.Store.DB, int(id))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, p)
}

func (h *HTTP) saveWorkflowPolicy(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	var req struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		WorkflowPolicy
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedRevision == nil || validateWorkflowPolicy(req.WorkflowPolicy) != nil {
		c.JSON(400, gin.H{"error": "invalid workflow policy or missing expected_revision"})
		return
	}
	p, err := h.Store.SaveWorkflowPolicy(c.Request.Context(), int(id), currentUser(c).ID, *req.ExpectedRevision, req.WorkflowPolicy)
	if err == ErrWorkflowPolicy {
		c.JSON(400, gin.H{"error": "invalid workflow policy"})
		return
	}
	if err == ErrConflict {
		c.JSON(409, gin.H{"error": "workflow policy changed; reload before saving", "current": p})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, p)
}
