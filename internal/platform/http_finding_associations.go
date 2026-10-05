package platform

import (
	"errors"
	"github.com/gin-gonic/gin"
)

func (h *HTTP) findingAssociations(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := h.Store.FindingAssociations(c.Request.Context(), id, currentUser(c).ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, items)
}
func (h *HTTP) decideFindingAssociation(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req AssociationDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid association decision"})
		return
	}
	decision, err := h.Store.SaveFindingAssociation(c.Request.Context(), id, currentUser(c).ID, c.Param("association_id"), req)
	if errors.Is(err, ErrAssociationDecision) {
		c.JSON(400, gin.H{"error": "invalid association decision"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, decision)
}
