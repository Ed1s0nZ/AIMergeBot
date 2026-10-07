package platform

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *HTTP) disposition(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	out, err := h.Store.Disposition(c.Request.Context(), id, currentUser(c).ID, c.Param("finding_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, out)
}
func (h *HTTP) saveDisposition(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req DispositionRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid finding disposition"})
		return
	}
	out, err := h.Store.SaveDisposition(c.Request.Context(), id, currentUser(c).ID, c.Param("finding_id"), req)
	if errors.Is(err, ErrDispositionInput) {
		c.JSON(400, gin.H{"error": "risk acceptance requires reason, authorized owner and future expiry; resolution requires explicit manual confirmation"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, out)
}
