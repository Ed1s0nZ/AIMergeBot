package platform

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *HTTP) ownerRouting(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	p, err := h.Store.OwnerRouting(c.Request.Context(), int(id), currentUser(c).ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *HTTP) saveOwnerRouting(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	var req struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		OwnerRouting
	}
	if c.ShouldBindJSON(&req) != nil || req.ExpectedRevision == nil || *req.ExpectedRevision < 0 || *req.ExpectedRevision == 1<<63-1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid owner routing or missing expected_revision"})
		return
	}
	if _, err := validateOwnerRouting(req.OwnerRouting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid owner routing"})
		return
	}
	p, err := h.Store.SaveOwnerRouting(c.Request.Context(), int(id), currentUser(c).ID, *req.ExpectedRevision, req.OwnerRouting)
	if err == ErrConflict {
		c.JSON(http.StatusConflict, gin.H{"error": "owner routing changed; reload before saving"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}
