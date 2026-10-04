package platform

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xanzy/go-gitlab"
)

func fail(c *gin.Context, err error) {
	status := 500
	message := "operation failed"
	if errors.Is(err, sql.ErrNoRows) {
		status = 404
		message = "not found"
	}
	if errors.Is(err, ErrConflict) || errors.Is(err, ErrLastAdmin) {
		status = 409
		message = err.Error()
	}
	c.JSON(status, gin.H{"error": message})
}
func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}
func pagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
func (h *HTTP) projects(c *gin.Context) {
	items, err := h.Store.Projects(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (h *HTTP) saveProject(c *gin.Context) {
	var p Project
	if c.ShouldBindJSON(&p) != nil {
		c.JSON(400, gin.H{"error": "invalid project"})
		return
	}
	if c.Param("id") != "" {
		id, ok := idParam(c)
		if !ok {
			return
		}
		p.ID = int(id)
	}
	if err := h.Store.SaveProject(c.Request.Context(), p); err != nil {
		c.JSON(400, gin.H{"error": "invalid project"})
		return
	}
	if h.Settings != nil {
		err := h.Store.SyncProjectConfig(c.Request.Context(), h.Settings)
		if err != nil {
			c.JSON(500, gin.H{"error": "project saved, but config.yaml synchronization failed; retry saving"})
			return
		}
	}
	h.Store.Event(c.Request.Context(), currentUser(c).ID, "project.saved", fmt.Sprint(p.ID))
	c.JSON(200, p)
}
func (h *HTTP) users(c *gin.Context) {
	items, err := h.Store.Users(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (h *HTTP) createUser(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid user"})
		return
	}
	u, err := h.Store.CreateUser(c.Request.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid or existing user; password needs 12–72 bytes"})
		return
	}
	h.Store.Event(c.Request.Context(), currentUser(c).ID, "user.created", fmt.Sprint(u.ID))
	c.JSON(201, u)
}
func (h *HTTP) updateUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid user"})
		return
	}
	if err := h.Store.UpdateUser(c.Request.Context(), id, req.Role, req.Disabled, req.Password); err != nil {
		fail(c, err)
		return
	}
	h.Store.Event(c.Request.Context(), currentUser(c).ID, "user.updated", fmt.Sprint(id))
	c.Status(204)
}
func (h *HTTP) submit(c *gin.Context) {
	var req struct {
		ProjectID int  `json:"project_id"`
		MRIID     int  `json:"mr_iid"`
		Force     bool `json:"force"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ProjectID <= 0 || req.MRIID <= 0 {
		c.JSON(400, gin.H{"error": "positive project_id and mr_iid required"})
		return
	}
	id, created, err := h.Runner.Submit(c.Request.Context(), req.ProjectID, req.MRIID, currentUser(c).ID, req.Force)
	if err != nil {
		c.JSON(422, gin.H{"error": "cannot enqueue audit; check project and GitLab configuration"})
		return
	}
	c.JSON(202, gin.H{"id": id, "created": created})
}
func (h *HTTP) run(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	r, err := h.Store.Run(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	reviews, err := h.Store.Reviews(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"run": r, "reviews": reviews})
}
func (h *HTTP) cancelRun(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.Runner.Cancel(c.Request.Context(), id, currentUser(c).ID); err != nil {
		fail(c, err)
		return
	}
	c.Status(204)
}
func (h *HTTP) review(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req Review
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid review"})
		return
	}
	req.RunID = id
	req.FindingID = c.Param("finding_id")
	req.Actor = currentUser(c).ID
	if err := h.Store.SaveReview(c.Request.Context(), req); err != nil {
		fail(c, err)
		return
	}
	c.Status(204)
}

func (h *HTTP) runs(c *gin.Context) {
	page, size := pagination(c)
	where := " WHERE 1=1"
	args := []any{}
	if p := c.Query("project_id"); p != "" {
		id, err := strconv.Atoi(p)
		if err != nil || id <= 0 {
			c.JSON(400, gin.H{"error": "invalid project_id"})
			return
		}
		where += " AND project_id=?"
		args = append(args, id)
	}
	if status := c.Query("status"); status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	if level := c.Query("level"); level != "" {
		where += ` AND EXISTS(SELECT 1 FROM json_each(platform_runs.result_json,'$.findings') f WHERE json_extract(f.value,'$.severity')=?)`
		args = append(args, level)
	}
	if kind := c.Query("type"); kind != "" {
		where += ` AND EXISTS(SELECT 1 FROM json_each(platform_runs.result_json,'$.findings') f WHERE json_extract(f.value,'$.type')=?)`
		args = append(args, kind)
	}
	if review := c.Query("review_status"); review != "" {
		if review == "pending" {
			where += ` AND EXISTS(SELECT 1 FROM json_each(platform_runs.result_json,'$.findings') f LEFT JOIN platform_reviews r ON r.run_id=platform_runs.id AND r.finding_id=json_extract(f.value,'$.id') WHERE COALESCE(r.status,'pending')='pending')`
		} else {
			where += ` AND EXISTS(SELECT 1 FROM platform_reviews r WHERE r.run_id=platform_runs.id AND r.status=?)`
			args = append(args, review)
		}
	}
	var total int
	if err := h.Store.DB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM platform_runs`+where, args...).Scan(&total); err != nil {
		fail(c, err)
		return
	}
	params := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := h.Store.DB.QueryContext(c.Request.Context(), `SELECT id FROM platform_runs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, params...)
	if err != nil {
		fail(c, err)
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			fail(c, err)
			return
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(c, err)
		return
	}
	items := []Run{}
	for _, id := range ids {
		r, err := h.Store.RunSummary(c.Request.Context(), id)
		if err != nil {
			fail(c, err)
			return
		}
		r.Trace = nil
		items = append(items, r)
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page, "size": size})
}
func (h *HTTP) events(c *gin.Context) {
	page, size := pagination(c)
	var total int
	if err := h.Store.DB.QueryRowContext(c.Request.Context(), `SELECT COUNT(*) FROM platform_events`).Scan(&total); err != nil {
		fail(c, err)
		return
	}
	rows, err := h.Store.DB.QueryContext(c.Request.Context(), `SELECT id,actor,action,target,created_at FROM platform_events ORDER BY id DESC LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var id, actor int64
		var action, target, created string
		if err = rows.Scan(&id, &actor, &action, &target, &created); err != nil {
			fail(c, err)
			return
		}
		items = append(items, gin.H{"id": id, "actor": actor, "action": action, "target": target, "created_at": created})
	}
	if err = rows.Err(); err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "page": page, "size": size, "total": total})
}
func (h *HTTP) webhook(c *gin.Context) {
	token := h.WebhookToken
	if h.Settings != nil {
		cfg := h.Settings.Snapshot()
		if !cfg.EnableWebhook {
			c.JSON(403, gin.H{"error": "webhook disabled"})
			return
		}
		token = cfg.WebhookToken
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Gitlab-Token")), []byte(token)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook token"})
		return
	}
	payload, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid payload"})
		return
	}
	event, err := gitlab.ParseWebhook(gitlab.HookEventType(c.Request), payload)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid GitLab event"})
		return
	}
	mr, ok := event.(*gitlab.MergeEvent)
	if !ok {
		c.JSON(200, gin.H{"ignored": true})
		return
	}
	action := mr.ObjectAttributes.Action
	if action != "open" && action != "update" && action != "reopen" && action != "synchronize" {
		c.JSON(200, gin.H{"ignored": true})
		return
	}
	id, created, err := h.Runner.Submit(c.Request.Context(), mr.Project.ID, mr.ObjectAttributes.IID, 0, false)
	if err != nil {
		c.JSON(422, gin.H{"error": "unable to enqueue configured project"})
		return
	}
	c.JSON(202, gin.H{"id": id, "created": created})
}

func (h *HTTP) getSettings(c *gin.Context) {
	if h.Settings == nil {
		c.JSON(503, gin.H{"error": "settings unavailable"})
		return
	}
	status, err := h.Store.ProjectSyncStatus(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	public := h.publicSettings()
	public["project_config_sync"] = status
	c.JSON(200, public)
}
func (h *HTTP) saveSettings(c *gin.Context) {
	if h.Settings == nil {
		c.JSON(503, gin.H{"error": "settings unavailable"})
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid settings body"})
		return
	}
	cfg, err := h.Settings.DecodePublic(raw)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid settings fields"})
		return
	}
	if err = h.Settings.Save(cfg); err != nil {
		status := 400
		if errors.Is(err, ErrTrustedProxies) {
			c.JSON(status, gin.H{"error": err.Error(), "code": "invalid_trusted_proxies"})
			return
		}
		if errors.Is(err, ErrSettingsConflict) {
			status = 409
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	h.Store.Event(c.Request.Context(), currentUser(c).ID, "settings.saved", "config.yaml")
	c.JSON(200, gin.H{"settings": h.publicSettings(), "restart_required": h.settingsRequireRestart(h.Settings.Snapshot()), "message": "saved; listen, worker count and trusted proxies require restart, model settings apply to new audits"})
}
