package platform

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *HTTP) findingTicketChannels(c *gin.Context) {
	runID, ok := idParam(c)
	if !ok {
		return
	}
	out, err := h.Store.FindingTicketChannels(c.Request.Context(), runID, currentUser(c).ID, c.Param("finding_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": out})
}

func (h *HTTP) findingTicket(c *gin.Context) {
	runID, ok := idParam(c)
	if !ok {
		return
	}
	integrationID, err := strconv.ParseInt(c.Param("integration_id"), 10, 64)
	if err != nil || integrationID <= 0 {
		c.JSON(400, gin.H{"error": "invalid integration identity"})
		return
	}
	out, err := h.Store.FindingTicket(c.Request.Context(), runID, currentUser(c).ID, integrationID, c.Param("finding_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, out)
}

func (h *HTTP) reserveFindingTicket(c *gin.Context) {
	runID, ok := idParam(c)
	if !ok {
		return
	}
	var req struct {
		IntegrationID    int64  `json:"integration_id"`
		ExpectedRevision *int64 `json:"expected_revision"`
		HeadSHA          string `json:"head_sha"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	if c.ShouldBindJSON(&req) != nil || req.IntegrationID <= 0 || req.ExpectedRevision == nil || *req.ExpectedRevision <= 0 || !commitID.MatchString(req.HeadSHA) {
		c.JSON(400, gin.H{"error": "integration identity, revision and commit required"})
		return
	}
	out, created, err := h.Store.ReserveFindingTicket(c.Request.Context(), runID, currentUser(c).ID, req.IntegrationID, *req.ExpectedRevision, c.Param("finding_id"), req.HeadSHA)
	if err != nil {
		fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	c.JSON(status, gin.H{"ticket": out, "reserved": created})
}
