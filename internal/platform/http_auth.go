package platform

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookie = "aim_session"

type loginBucket struct {
	Count int
	Reset time.Time
}
type HTTP struct {
	Settings        *SettingsService
	Store           *Store
	Runner          *Runner
	WebhookToken    string
	mu              sync.Mutex
	loginAttempts   map[loginKey]loginBucket
	startupSettings *Settings
}

func (h *HTTP) origin(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		return true
	}
	if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if h.Settings != nil {
		cfg := h.Settings.Snapshot()
		if cfg.PublicURL != "" {
			expected, err := url.Parse(cfg.PublicURL)
			return err == nil && u.Host == expected.Host && u.Scheme == expected.Scheme
		}
	}
	return u.Host == c.Request.Host && u.Scheme == scheme
}
func (h *HTTP) guard(c *gin.Context) {
	if !h.origin(c) {
		c.AbortWithStatusJSON(403, gin.H{"error": "cross-origin write rejected"})
		return
	}
	token, err := c.Cookie(sessionCookie)
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"error": "login required"})
		return
	}
	u, err := h.Store.Session(c.Request.Context(), token)
	if err != nil {
		status := 500
		if errors.Is(err, ErrCredentials) {
			status = 401
		}
		c.AbortWithStatusJSON(status, gin.H{"error": "session unavailable"})
		return
	}
	c.Set("user", u)
	c.Next()
}
func currentUser(c *gin.Context) User { return c.MustGet("user").(User) }
func admin(c *gin.Context) {
	if currentUser(c).Role != "admin" {
		c.AbortWithStatusJSON(403, gin.H{"error": "administrator required"})
		return
	}
	c.Next()
}
func (h *HTTP) cookie(c *gin.Context, token string, maxAge int) {
	secure := c.Request.TLS != nil
	if h.Settings != nil {
		secure = secure || strings.HasPrefix(h.Settings.Snapshot().PublicURL, "https://")
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, maxAge, "/", "", secure, true)
}
func (h *HTTP) login(c *gin.Context) {
	if !h.origin(c) {
		c.JSON(403, gin.H{"error": "cross-origin write rejected"})
		return
	}
	ip := c.ClientIP()
	reject := func(wait time.Duration) bool {
		if wait <= 0 {
			return false
		}
		c.Header("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
		c.JSON(429, gin.H{"error": "too many login attempts; retry later"})
		return true
	}
	if reject(h.admitLogin(ip, "", false, time.Now())) {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Username) > 64 || len(req.Password) > 72 {
		c.JSON(400, gin.H{"error": "invalid login request"})
		return
	}
	if reject(h.admitLogin(ip, req.Username, true, time.Now())) {
		return
	}
	u, token, err := h.Store.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, ErrCredentials) {
			c.JSON(401, gin.H{"error": "invalid credentials"})
		} else {
			c.JSON(500, gin.H{"error": "login unavailable"})
		}
		return
	}
	h.loginSucceeded(ip, req.Username)
	h.cookie(c, token, 86400)
	c.JSON(200, u)
}

func (h *HTTP) Register(r *gin.Engine) {
	if h.Settings != nil {
		h.Store.BindQuotaSettings(h.Settings)
		cfg := h.Settings.Snapshot()
		h.startupSettings = &cfg
	}
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) {
		if h.Runner != nil && h.Runner.workerStopped() {
			c.JSON(503, gin.H{"status": "worker_unavailable"})
			return
		}
		if h.Store.DB.PingContext(c.Request.Context()) != nil {
			c.JSON(503, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/readyz", h.ready)
	r.POST("/api/v1/auth/login", h.login)
	r.POST("/api/v1/bot-callbacks/slack/:id/:revision/bind", h.slackBindingCallback)
	r.POST("/api/v1/bot-callbacks/slack/:id/:revision/command", h.slackBindingCallback)
	api := r.Group("/api/v1", h.guard)
	api.GET("/auth/me", func(c *gin.Context) { c.JSON(200, currentUser(c)) })
	api.POST("/bot-bindings/:id/challenge", h.issueBotBinding)
	api.GET("/bot-bindings", h.ownBotBindings)
	api.GET("/bot-binding-channels", h.botBindingChannels)
	api.DELETE("/bot-bindings/:id", h.revokeBotBinding)
	api.POST("/auth/logout", func(c *gin.Context) {
		token, _ := c.Cookie(sessionCookie)
		if h.Store.Logout(c.Request.Context(), token) != nil {
			c.JSON(500, gin.H{"error": "logout failed"})
			return
		}
		h.cookie(c, "", -1)
		c.Status(204)
	})
	api.GET("/deliveries", admin, h.notificationRecords)
	api.POST("/deliveries/:id/retry", admin, h.retryNotification)
	api.POST("/integrations/:id/test", admin, h.testIntegration)
	api.GET("/integrations", admin, h.integrations)
	api.POST("/integrations", admin, h.saveIntegration)
	api.PATCH("/integrations/:id", admin, h.saveIntegration)
	api.GET("/settings", admin, h.getSettings)
	api.PUT("/settings", admin, h.saveSettings)
	api.GET("/projects", h.projects)
	api.POST("/projects", admin, h.saveProject)
	api.POST("/repository-projects", admin, h.createRepositoryProject)
	api.PATCH("/projects/:id", admin, h.saveProject)
	api.GET("/projects/:id/repository-binding", h.repositoryBinding)
	api.PATCH("/projects/:id/repository-binding", admin, h.saveRepositoryBinding)
	api.GET("/projects/:id/workflow-policy", admin, h.workflowPolicy)
	api.PUT("/projects/:id/workflow-policy", admin, h.saveWorkflowPolicy)
	api.GET("/projects/:id/owner-routing", h.ownerRouting)
	api.PUT("/projects/:id/owner-routing", admin, h.saveOwnerRouting)
	api.GET("/projects/:id/context-repositories", admin, h.contextRepositories)
	api.PUT("/projects/:id/context-repositories", admin, h.saveContextRepositories)
	api.GET("/projects/:id/members", admin, h.projectMembers)
	api.PUT("/projects/:id/members/:user_id", admin, h.setProjectMember)
	api.DELETE("/projects/:id/members/:user_id", admin, h.setProjectMember)
	api.GET("/users", admin, h.users)
	api.POST("/users", admin, h.createUser)
	api.PATCH("/users/:id", admin, h.updateUser)
	api.GET("/workspace/usage", h.workspaceUsage)
	api.GET("/workspace/quality", h.workspaceQuality)
	api.GET("/workspace/findings", h.workspaceFindings)
	api.GET("/workspace/tasks", h.workspaceTasks)
	api.GET("/runs", h.runs)
	api.POST("/runs", h.submit)
	api.GET("/runs/:id", h.run)
	api.GET("/runs/:id/status", h.runStatus)
	api.GET("/runs/:id/sarif", h.runSARIF)
	api.GET("/runs/:id/scope", h.runScope)
	api.GET("/runs/:id/comparison", h.compareRuns)
	api.GET("/runs/:id/associations", h.findingAssociations)
	api.PUT("/runs/:id/associations/:association_id", h.decideFindingAssociation)
	api.POST("/runs/:id/followup", h.followupRun)
	api.POST("/runs/:id/cancel", h.cancelRun)
	api.GET("/runs/:id/findings/:finding_id/disposition", h.disposition)
	api.GET("/runs/:id/findings/:finding_id/owners", h.findingOwners)
	api.PUT("/runs/:id/findings/:finding_id/disposition", h.saveDisposition)
	api.POST("/runs/:id/findings/:finding_id/tickets", h.reserveFindingTicket)
	api.GET("/runs/:id/findings/:finding_id/tickets", h.findingTickets)
	api.GET("/runs/:id/findings/:finding_id/ticket-channels", h.findingTicketChannels)
	api.GET("/runs/:id/findings/:finding_id/tickets/:integration_id", h.findingTicket)
	api.PUT("/runs/:id/findings/:finding_id/review", h.review)
	api.GET("/events", admin, h.events)
	r.POST("/webhook", h.webhook)
}
