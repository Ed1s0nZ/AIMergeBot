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
	if errors.Is(err, ErrWorkerLeaseLost) {
		c.Header("Retry-After", "30")
		c.JSON(503, gin.H{"error": "audit worker unavailable", "code": "worker_unavailable"})
		return
	}
	var quota *QuotaError
	if errors.As(err, &quota) {
		c.Header("Retry-After", "60")
		c.JSON(429, gin.H{"error": "audit capacity reached; retry later", "code": "audit_quota_exceeded", "scope": quota.Scope, "limit": quota.Limit})
		return
	}
	status := 500
	message := "operation failed"
	code := ""
	if errors.Is(err, ErrCredentials) {
		status = 401
		message = "session unavailable"
	}
	if errors.Is(err, ErrProjectPermission) {
		status = 403
		message = "project permission required"
		code = "project_permission_required"
	}
	if errors.Is(err, sql.ErrNoRows) {
		status = 404
		message = "not found"
		code = "not_found"
	}
	if errors.Is(err, ErrConflict) || errors.Is(err, ErrLastAdmin) {
		status = 409
		message = err.Error()
	}
	c.JSON(status, gin.H{"error": message, "code": code})
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
	items, err := h.Store.ProjectsForUser(c.Request.Context(), currentUser(c))
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
	if _, ok := h.requireProject(c, req.ProjectID, "operator"); !ok {
		return
	}
	id, created, err := h.Runner.Submit(c.Request.Context(), req.ProjectID, req.MRIID, currentUser(c).ID, req.Force)
	if err != nil {
		var quota *QuotaError
		if errors.Is(err, ErrWorkerLeaseLost) || errors.As(err, &quota) || errors.Is(err, ErrProjectPermission) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrCredentials) {
			fail(c, err)
			return
		}
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
	role, allowed := h.requireRun(c, id, "viewer")
	if !allowed {
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
	access := permissions(role)
	var enabled bool
	if err = h.Store.DB.QueryRowContext(c.Request.Context(), `SELECT enabled FROM platform_projects WHERE id=?`, r.ProjectID).Scan(&enabled); err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(c, err)
		return
	}
	access.CanSubmit = access.CanSubmit && enabled
	wait, err := h.Store.QueueWaitFor(c.Request.Context(), r)
	if err != nil {
		fail(c, err)
		return
	}
	var syncState any
	delivery, deliveryErr := h.Store.CommentDelivery(c.Request.Context(), id)
	if deliveryErr == nil {
		enabled := h.Runner != nil && h.Runner.Settings != nil && h.Runner.Settings.Snapshot().EnableMRComment
		syncState = struct {
			CommentDelivery
			Enabled        bool `json:"enabled"`
			RetryExhausted bool `json:"retry_exhausted"`
		}{delivery, enabled, delivery.Attempts >= 5}
	} else if !errors.Is(deliveryErr, sql.ErrNoRows) {
		fail(c, deliveryErr)
		return
	}
	lifecycle, err := h.Store.FindingLifecycle(c.Request.Context(), r)
	if err != nil {
		fail(c, err)
		return
	}
	budget := ModelBudgetSettings{}
	differentVerifier := false
	if r.AuditPolicy != nil {
		budget = r.AuditPolicy.ModelBudget
		differentVerifier = r.AuditPolicy.VerificationModel != "" && r.AuditPolicy.VerificationModel != r.AuditPolicy.Model
	}
	usage := SummarizeModelUsage(r.Trace, budget, (r.Status == "succeeded" || r.Status == "incomplete") && r.Error == "", differentVerifier)
	c.JSON(200, gin.H{"usage": usage, "finding_lifecycle": lifecycle, "run": r, "reviews": reviews, "permissions": access, "queue_wait": wait, "comment_sync": syncState})
}
func (h *HTTP) cancelRun(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if _, ok := h.requireRun(c, id, "operator"); !ok {
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
	if _, ok := h.requireRun(c, id, "reviewer"); !ok {
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
	if err := h.Store.SaveReviewUser(c.Request.Context(), req); err != nil {
		fail(c, err)
		return
	}
	c.Status(204)
}

func (h *HTTP) runs(c *gin.Context) {
	page, size := pagination(c)
	where := " WHERE 1=1"
	args := []any{}
	if user := currentUser(c); user.Role != "admin" {
		where += ` AND EXISTS(SELECT 1 FROM platform_project_members m WHERE m.project_id=platform_runs.project_id AND m.user_id=?)`
		args = append(args, user.ID)
		where += ` AND (source_project_id=project_id OR EXISTS(SELECT 1 FROM platform_project_members src WHERE src.project_id=platform_runs.source_project_id AND src.user_id=?))`
		args = append(args, user.ID)
	}
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
		where += ` AND EXISTS(SELECT 1 FROM platform_finding_index f WHERE f.run_id=platform_runs.id AND f.severity=?)`
		args = append(args, level)
	}
	if kind := c.Query("type"); kind != "" {
		where += ` AND EXISTS(SELECT 1 FROM platform_finding_index f WHERE f.run_id=platform_runs.id AND f.kind=?)`
		args = append(args, kind)
	}
	if review := c.Query("review_status"); review != "" {
		if review == "pending" {
			where += ` AND EXISTS(SELECT 1 FROM platform_finding_index f LEFT JOIN platform_reviews r ON r.run_id=f.run_id AND r.finding_id=f.finding_id WHERE f.run_id=platform_runs.id AND COALESCE(r.status,'pending')='pending')`
		} else {
			where += ` AND EXISTS(SELECT 1 FROM platform_finding_index f JOIN platform_reviews r ON r.run_id=f.run_id AND r.finding_id=f.finding_id WHERE f.run_id=platform_runs.id AND r.status=?)`
			args = append(args, review)
		}
	}
	items, total, err := h.Store.listRuns(c.Request.Context(), where, args, page, size)
	if err != nil {
		fail(c, err)
		return
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
		var quota *QuotaError
		if errors.Is(err, ErrWorkerLeaseLost) || errors.As(err, &quota) {
			fail(c, err)
			return
		}
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
		if errors.Is(err, ErrAuditQuotas) {
			c.JSON(400, gin.H{"error": err.Error(), "code": "invalid_audit_quotas"})
			return
		}
		c.JSON(400, gin.H{"error": "invalid settings fields"})
		return
	}
	if err = h.Settings.Save(cfg); err != nil {
		status := 400
		if errors.Is(err, ErrAuditQuotas) {
			c.JSON(status, gin.H{"error": err.Error(), "code": "invalid_audit_quotas"})
			return
		}
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
