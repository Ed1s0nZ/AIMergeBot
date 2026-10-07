package platform

import "github.com/gin-gonic/gin"

func (h *HTTP) findingOwners(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var repository Repository
	if h.Runner != nil {
		repository = h.Runner.Repository
	}
	report, err := h.Store.RecommendFindingOwners(c.Request.Context(), id, currentUser(c).ID, c.Param("finding_id"), repository)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, report)
}
