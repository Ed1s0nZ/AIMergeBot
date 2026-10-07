package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type MetricsWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func metricsScope(c *gin.Context) (string, []any, MetricsWindow, bool) {
	at := time.Now().UTC()
	from := at.AddDate(0, 0, -30)
	to := at
	for _, item := range []struct {
		key    string
		target *time.Time
	}{{"from", &from}, {"to", &to}} {
		if value := c.Query(item.key); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				c.JSON(400, gin.H{"error": "invalid time window"})
				return "", nil, MetricsWindow{}, false
			}
			*item.target = parsed.UTC()
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		c.JSON(400, gin.H{"error": "time window must be positive and at most 366 days"})
		return "", nil, MetricsWindow{}, false
	}
	where, args := workspaceACL(currentUser(c))
	where += " AND julianday(platform_runs.created_at)>=julianday(?) AND julianday(platform_runs.created_at)<julianday(?)"
	window := MetricsWindow{from.Format(time.RFC3339), to.Format(time.RFC3339)}
	args = append(args, window.From, window.To)
	if value := c.Query("project_id"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			c.JSON(400, gin.H{"error": "invalid project_id"})
			return "", nil, window, false
		}
		where += " AND platform_runs.project_id=?"
		args = append(args, id)
	}
	return where, args, window, true
}

type WorkspaceUsage struct {
	RunID           int64       `json:"run_id"`
	ProjectID       int         `json:"project_id"`
	MRIID           int         `json:"mr_iid"`
	HeadSHA         string      `json:"head_sha"`
	Model           string      `json:"model"`
	Status          string      `json:"status"`
	CreatedAt       string      `json:"created_at"`
	DurationSeconds *float64    `json:"duration_seconds"`
	Usage           *ModelUsage `json:"usage"`
	UnknownReason   string      `json:"unknown_reason,omitempty"`
}

func (s *Store) workspaceUsage(ctx context.Context, where string, args []any, page, size int) ([]WorkspaceUsage, int, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_runs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	params := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := tx.QueryContext(ctx, `SELECT id,project_id,mr_iid,head_sha,status,error,created_at,started_at,finished_at,audit_policy_json,length(CAST(trace_json AS BLOB)),CASE WHEN length(CAST(trace_json AS BLOB))<=1048576 THEN trace_json ELSE '' END FROM platform_runs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, params...)
	if err != nil {
		return nil, 0, err
	}
	items := []WorkspaceUsage{}
	for rows.Next() {
		var item WorkspaceUsage
		var started, finished sql.NullString
		var policy, trace, runError string
		var traceSize int
		if err = rows.Scan(&item.RunID, &item.ProjectID, &item.MRIID, &item.HeadSHA, &item.Status, &runError, &item.CreatedAt, &started, &finished, &policy, &traceSize, &trace); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if started.Valid && finished.Valid {
			a, e1 := time.Parse(time.RFC3339Nano, started.String)
			b, e2 := time.Parse(time.RFC3339Nano, finished.String)
			if e1 == nil && e2 == nil && !b.Before(a) {
				seconds := b.Sub(a).Seconds()
				item.DurationSeconds = &seconds
			}
		}
		var p *AuditPolicy
		var tr []ToolTrace
		item.Model = "unknown"
		policyErr := json.Unmarshal([]byte(policy), &p)
		if policyErr == nil && p != nil && p.Model != "" {
			item.Model = p.Model
		}
		if traceSize > 1048576 {
			item.UnknownReason = "trace_exceeds_read_budget"
		} else if policyErr != nil || json.Unmarshal([]byte(trace), &tr) != nil {
			item.UnknownReason = "invalid_historical_receipt"
		} else {
			budget := ModelBudgetSettings{}
			different := false
			if p != nil {
				budget = p.ModelBudget
				different = p.VerificationModel != "" && p.VerificationModel != p.Model
			}
			usage := SummarizeModelUsage(tr, budget, (item.Status == "succeeded" || item.Status == "incomplete") && runError == "" && len(tr) > 0, different)
			item.Usage = &usage
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	return items, total, tx.Commit()
}
func (h *HTTP) workspaceUsage(c *gin.Context) {
	where, args, window, ok := metricsScope(c)
	if !ok {
		return
	}
	page, size := pagination(c)
	if size > 20 {
		size = 20
	}
	items, total, err := h.Store.workspaceUsage(c.Request.Context(), where, args, page, size)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page, "size": size, "window": window})
}

type QualityFeedback struct {
	Model         string `json:"model"`
	Policy        string `json:"policy"`
	Snapshot      string `json:"snapshot"`
	Findings      int    `json:"findings"`
	Pending       int    `json:"pending"`
	Accepted      int    `json:"accepted"`
	Fixed         int    `json:"fixed"`
	FalsePositive int    `json:"false_positive"`
	Other         int    `json:"other"`
}

func (s *Store) workspaceQuality(ctx context.Context, where string, args []any) ([]QualityFeedback, bool, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(NULLIF(CASE WHEN json_valid(audit_policy_json) THEN json_extract(audit_policy_json,'$.model') END,''),'unknown'),policy_version,audit_policy_json,COUNT(*),SUM(CASE WHEN COALESCE(rv.status,'pending')='pending' THEN 1 ELSE 0 END),SUM(CASE WHEN rv.status='accepted' THEN 1 ELSE 0 END),SUM(CASE WHEN rv.status='fixed' THEN 1 ELSE 0 END),SUM(CASE WHEN rv.status='false_positive' THEN 1 ELSE 0 END),SUM(CASE WHEN rv.status IS NOT NULL AND rv.status NOT IN ('pending','accepted','fixed','false_positive') THEN 1 ELSE 0 END) FROM platform_runs JOIN platform_finding_index f ON f.run_id=platform_runs.id LEFT JOIN platform_reviews rv ON rv.run_id=f.run_id AND rv.finding_id=f.finding_id`+where+` GROUP BY 1,2,3 ORDER BY 1,2,3 LIMIT 501`, args...)
	if err != nil {
		return nil, false, err
	}
	items := []QualityFeedback{}
	for rows.Next() {
		var item QualityFeedback
		var rawPolicy string
		if err = rows.Scan(&item.Model, &item.Policy, &rawPolicy, &item.Findings, &item.Pending, &item.Accepted, &item.Fixed, &item.FalsePositive, &item.Other); err != nil {
			rows.Close()
			return nil, false, err
		}
		sum := sha256.Sum256([]byte(rawPolicy))
		item.Snapshot = hex.EncodeToString(sum[:])
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	truncated := len(items) > 500
	if truncated {
		items = items[:500]
	}
	return items, truncated, tx.Commit()
}
func (h *HTTP) workspaceQuality(c *gin.Context) {
	where, args, window, ok := metricsScope(c)
	if !ok {
		return
	}
	items, truncated, err := h.Store.workspaceQuality(c.Request.Context(), where, args)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "window": window, "truncated": truncated, "limitations": []string{"人工反馈不是独立评测真值；未复核不计正确或误报。", "不同模型或策略可能审计不同提交，不能据此直接排名。", "尚未登记漏报及独立真值，不能计算召回率。"}})
}
