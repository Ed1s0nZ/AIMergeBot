package platform

import (
	"encoding/json"
	"errors"
	"io"
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

func (h *HTTP) saveRepositoryBinding(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		Provider         string `json:"provider"`
		APIOrigin        string `json:"api_origin"`
		RemoteID         int64  `json:"remote_id"`
		FullName         string `json:"full_name"`
		IntegrationID    int64  `json:"integration_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.ExpectedRevision == nil || *req.ExpectedRevision < 0 || *req.ExpectedRevision == 1<<63-1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository binding or missing expected_revision"})
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository binding"})
		return
	}
	binding := RepositoryBinding{Provider: req.Provider, APIOrigin: req.APIOrigin, RemoteID: req.RemoteID, FullName: req.FullName, IntegrationID: req.IntegrationID}
	if _, err := validateRepositoryBinding(binding); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository binding"})
		return
	}
	saved, err := h.Store.SaveRepositoryBinding(c.Request.Context(), int(id), currentUser(c).ID, *req.ExpectedRevision, binding)
	if errors.Is(err, ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "repository binding changed or unavailable; reload and verify configuration"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, saved)
}
