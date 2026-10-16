package platform

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) repositoryBinding(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	binding, err := h.Store.RepositoryBinding(c.Request.Context(), int(id), currentUser(c).ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, binding)
}
