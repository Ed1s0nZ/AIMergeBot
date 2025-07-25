package internal

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xanzy/go-gitlab"
)

// 在import后添加类型别名
type MRAnalysisResult = struct {
	MRID          int             `json:"mr_id"`
	ProjectID     int             `json:"project_id"`
	ProjectName   string          `json:"project_name"`
	ProjectPath   string          `json:"project_path"`
	MRTitle       string          `json:"mr_title"`
	MRAuthor      string          `json:"mr_author"`
	MRCreated     string          `json:"mr_created"`
	MRBranch      string          `json:"mr_branch"`
	MRDesc        string          `json:"mr_desc"`
	MRUrl         string          `json:"mr_url"`
	GitLabBaseUrl string          `json:"gitlab_base_url"`
	Result        []SecurityIssue `json:"result"`
	ReviewStatus  string          `json:"review_status"` // 新增字段
}

// 注册 /results 路由，支持分页和过滤
func RegisterResultRoute(r *gin.Engine, storage *Storage) {
	r.GET("/results", func(c *gin.Context) {
		GetResultsHandler(c, storage)
	})
	// 新增项目列表接口
	r.GET("/projects", func(c *gin.Context) {
		projects, err := storage.GetAllProjectsFromResults()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询项目列表失败"})
			return
		}
		c.JSON(200, projects)
	})
	// 新增：审核状态更新接口
	r.POST("/mr_status", func(c *gin.Context) {
		log.Printf("POST /mr_status called")
		var req struct {
			ProjectID int    `json:"project_id"`
			MRID      int    `json:"mr_id"`
			Status    string `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("POST /mr_status param error: %v", err)
			c.JSON(400, gin.H{"error": "参数错误"})
			return
		}
		log.Printf("POST /mr_status param: project_id=%d, mr_id=%d, status=%s", req.ProjectID, req.MRID, req.Status)
		if err := storage.SetReviewStatus(req.ProjectID, req.MRID, req.Status); err != nil {
			log.Printf("POST /mr_status SetReviewStatus error: %v", err)
			c.JSON(500, gin.H{"error": "保存失败"})
			return
		}
		// 立即读取数据库最新状态
		status, err := storage.GetReviewStatus(req.ProjectID, req.MRID)
		log.Printf("POST /mr_status after write, db review_status=%s, err=%v", status, err)
		c.JSON(200, gin.H{"msg": "success", "review_status": status})
	})
}

// 支持分页和过滤的结果查询
func GetResultsHandler(c *gin.Context, storage *Storage) {
	pageStr := c.DefaultQuery("page", "1")
	sizeStr := c.DefaultQuery("size", "20")
	// projectIDStr := c.Query("project_id") // Remove unused variable
	level := c.Query("level")
	riskType := c.Query("type")

	page, _ := strconv.Atoi(pageStr)
	size, _ := strconv.Atoi(sizeStr)
	// projectID, _ := strconv.Atoi(projectIDStr) // Remove unused variable

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	// 查询所有结果
	allResults, err := storage.GetAllResults()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询结果失败"})
		return
	}
	// 为每个MR补充review_status字段
	for i := range allResults {
		status, _ := storage.GetReviewStatus(allResults[i].ProjectID, allResults[i].MRID)
		allResults[i].ReviewStatus = status
	}

	// 统计所有风险
	total, high, medium, low := 0, 0, 0, 0
	for _, r := range allResults {
		for _, issue := range r.Result {
			total++
			switch issue.Level {
			case "high":
				high++
			case "medium":
				medium++
			case "low":
				low++
			}
		}
	}

	// 统计所有类型和等级
	typeSet := map[string]struct{}{}
	levelSet := map[string]struct{}{}
	for _, r := range allResults {
		for _, issue := range r.Result {
			typeSet[issue.Type] = struct{}{}
			levelSet[issue.Level] = struct{}{}
		}
	}
	allTypes := []string{}
	for t := range typeSet {
		allTypes = append(allTypes, t)
	}
	allLevels := []string{}
	for l := range levelSet {
		allLevels = append(allLevels, l)
	}

	// 过滤
	filteredResults := []MRAnalysisResult{}
	for _, r := range allResults {
		// 审核状态筛选
		if reviewStatus := c.Query("review_status"); reviewStatus != "" && reviewStatus != "all" && r.ReviewStatus != reviewStatus {
			continue
		}
		// 风险筛选：只返回有风险的MR，除非明确筛选全部
		if len(r.Result) == 0 && (level != "" || riskType != "" || c.Query("review_status") != "" && c.Query("review_status") != "all") {
			// 如果有风险等级、类型、审核状态等筛选条件时，过滤掉无风险MR
			continue
		}
		filteredResults = append(filteredResults, r)
	}

	// 分页
	totalFiltered := len(filteredResults)
	start := (page - 1) * size
	end := start + size
	if start >= totalFiltered {
		c.JSON(200, gin.H{
			"results": []MRAnalysisResult{},
			"pagination": gin.H{
				"page":     page,
				"size":     size,
				"total":    totalFiltered,
				"pages":    0,
				"has_next": false,
				"has_prev": false,
			},
			"stats": gin.H{
				"total":  total,
				"high":   high,
				"medium": medium,
				"low":    low,
			},
			"all_types":  allTypes,
			"all_levels": allLevels,
		})
		return
	}
	if end > totalFiltered {
		end = totalFiltered
	}
	pagedResults := filteredResults[start:end]

	c.JSON(200, gin.H{
		"results": pagedResults,
		"pagination": gin.H{
			"page":     page,
			"size":     size,
			"total":    totalFiltered,
			"pages":    (totalFiltered + size - 1) / size,
			"has_next": end < totalFiltered,
			"has_prev": page > 1,
		},
		"stats": gin.H{
			"total":  total,
			"high":   high,
			"medium": medium,
			"low":    low,
		},
		"all_types":  allTypes,
		"all_levels": allLevels,
	})
}

// 过滤函数
func filterResults(results []MRAnalysisResult, projectID int, level, riskType string) []MRAnalysisResult {
	if projectID == 0 && level == "" && riskType == "" {
		return results
	}
	var filtered []MRAnalysisResult
	for _, result := range results {
		if projectID != 0 && result.ProjectID != projectID {
			continue
		}
		if level != "" {
			hasLevel := false
			for _, issue := range result.Result {
				if issue.Level == level {
					hasLevel = true
					break
				}
			}
			if !hasLevel {
				continue
			}
		}
		if riskType != "" {
			hasType := false
			for _, issue := range result.Result {
				if issue.Type == riskType {
					hasType = true
					break
				}
			}
			if !hasType {
				continue
			}
		}
		filtered = append(filtered, result)
	}
	return filtered
}

// WebhookHandler 由 main.go 调用，参数为 (cfg *Config, storage *Storage)
func WebhookHandler(cfg *Config, storage *Storage) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 读取 payload
		payload, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取 webhook payload 失败"})
			return
		}

		// 2. 解析事件类型
		eventType := gitlab.HookEventType(c.Request)
		event, err := gitlab.ParseWebhook(eventType, payload)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook 解析失败", "detail": err.Error()})
			return
		}

		// 3. 只处理 Merge Request 事件
		mergeEvent, ok := event.(*gitlab.MergeEvent)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"msg": "非 Merge Request 事件，已忽略"})
			return
		}

		// 4. 只处理 open/update/reopen/synchronize 事件
		action := mergeEvent.ObjectAttributes.Action
		if action != "open" && action != "update" && action != "reopen" && action != "synchronize" {
			c.JSON(http.StatusOK, gin.H{"msg": "MR 非变更事件，已忽略"})
			return
		}

		projectID := mergeEvent.Project.ID
		mrIID := mergeEvent.ObjectAttributes.IID

		// 5. 检查是否已分析过
		status, _ := storage.GetAnalyzedStatus(projectID, mrIID)
		if status == "processing" || status == "done" {
			c.JSON(http.StatusOK, gin.H{"msg": "该 MR 已分析过，跳过"})
			return
		}
		storage.SetAnalyzedStatus(projectID, mrIID, "processing")

		// 6. 拉取 MR diff
		git, err := gitlab.NewClient(cfg.GitLab.Token, gitlab.WithBaseURL(cfg.GitLab.URL+"/api/v4"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GitLab 客户端初始化失败"})
			return
		}
		diff, err := GetMRDiffWithWhitelist(git, projectID, mrIID, cfg.WhitelistExtensions)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 MR diff 失败", "detail": err.Error()})
			return
		}
		if diff == "" {
			log.Printf("[AIMergeBot] MR !%d 所有变更文件均为白名单，跳过分析", mrIID)
			c.JSON(http.StatusOK, gin.H{"msg": "MR 所有变更文件均为白名单，跳过分析"})
			return
		}

		// 7. AI 分析
		issues, err := AnalyzeDiffWithOpenAI(cfg.OpenAI.APIKey, diff, cfg.OpenAI.URL, cfg.OpenAI.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI 分析失败", "detail": err.Error()})
			return
		}

		// 8. 为每个安全问题生成修复建议
		for i := range issues {
			if fixSuggestion, err := generateFixSuggestion(cfg.OpenAI.APIKey, cfg.OpenAI.URL, cfg.OpenAI.Model, issues[i]); err == nil {
				issues[i].FixSuggestion = fixSuggestion
			} else {
				issues[i].FixSuggestion = "修复建议生成失败，请手动处理"
			}
		}

		// 9. 存储分析结果
		storage.SetAnalyzedStatus(projectID, mrIID, "done")
		storage.AddResult(MRAnalysisResult{
			MRID:          mrIID,
			ProjectID:     projectID,
			ProjectName:   mergeEvent.Project.Name,
			ProjectPath:   mergeEvent.Project.PathWithNamespace,
			MRTitle:       mergeEvent.ObjectAttributes.Title,
			MRAuthor:      mergeEvent.User.Username,
			MRCreated:     mergeEvent.ObjectAttributes.CreatedAt,
			MRBranch:      mergeEvent.ObjectAttributes.SourceBranch,
			MRDesc:        mergeEvent.ObjectAttributes.Description,
			MRUrl:         mergeEvent.ObjectAttributes.URL,
			GitLabBaseUrl: cfg.GitLab.URL,
			Result:        issues,
		})

		// 10. 自动评论
		if cfg.EnableMRComment {
			comment := formatMRComment(issues)
			_ = addMRComment(git, projectID, mrIID, comment)
		}

		c.JSON(http.StatusOK, gin.H{"msg": "MR 分析完成", "issues": issues})
	}
}
