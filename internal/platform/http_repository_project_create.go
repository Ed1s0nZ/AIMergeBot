package platform

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) createRepositoryProject(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var input RepositoryProjectInput
	if decoder.Decode(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository project configuration"})
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository project configuration"})
		return
	}
	receipt, err := h.Store.CreateRepositoryProject(c.Request.Context(), currentUser(c).ID, input)
	if errors.Is(err, ErrRepositoryProjectInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository project configuration"})
		return
	}
	if errors.Is(err, ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "repository project request or integration changed; reload configuration"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if h.Settings != nil {
		if err = h.Store.SyncProjectConfig(c.Request.Context(), h.Settings); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "repository project created; configuration synchronization pending; retry with the same request_id", "code": "project_config_sync_pending"})
			return
		}
	}
	status := http.StatusCreated
	if receipt.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, receipt)
}
