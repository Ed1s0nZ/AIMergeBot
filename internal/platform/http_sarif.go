package platform

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
)

func (h *HTTP) runSARIF(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, ok = h.requireRun(c, id, "viewer"); !ok {
		return
	}
	run, err := h.Store.Run(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	reviews, err := h.Store.Reviews(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	body, err := json.Marshal(BuildSARIF(run, reviews))
	if err != nil {
		fail(c, err)
		return
	}
	if _, ok = h.requireRun(c, id, "viewer"); !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="aimangebot-run-%d.sarif"`, id))
	c.Data(200, "application/sarif+json", body)
}
