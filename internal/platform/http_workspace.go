package platform

import (
	"context"
	"database/sql"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

type WorkspaceFinding struct {
	RunID        int64  `json:"run_id"`
	ProjectID    int64  `json:"project_id"`
	MRIID        int    `json:"mr_iid"`
	HeadSHA      string `json:"head_sha"`
	RunStatus    string `json:"run_status"`
	FindingID    string `json:"finding_id"`
	Severity     string `json:"severity"`
	Kind         string `json:"type"`
	Title        string `json:"title"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	ReviewStatus string `json:"review_status"`
	Disposition  string `json:"disposition"`
	Owner        int64  `json:"owner"`
	ExpiresAt    string `json:"expires_at"`
}

// Same ACL as task detail/list: target, fork source and all pinned contexts.
func workspaceACL(user User) (string, []any) {
	if user.Role == "admin" {
		return " WHERE 1=1", nil
	}
	return ` WHERE EXISTS(SELECT 1 FROM platform_project_members m WHERE m.project_id=platform_runs.project_id AND m.user_id=?) AND (source_project_id=project_id OR EXISTS(SELECT 1 FROM platform_project_members m WHERE m.project_id=source_project_id AND m.user_id=?))` + contextListACL, []any{user.ID, user.ID, user.ID}
}

func workspaceFilters(c *gin.Context, where string, args []any) (string, []any, bool) {
	if value := c.Query("project_id"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			c.JSON(400, gin.H{"error": "invalid project_id"})
			return where, args, false
		}
		where += " AND platform_runs.project_id=?"
		args = append(args, id)
	}
	for _, filter := range []struct{ key, column string }{{"severity", "f.severity"}, {"type", "f.kind"}, {"review_status", "COALESCE(rv.status,'pending')"}} {
		if value := c.Query(filter.key); value != "" {
			if len(value) > 80 {
				c.JSON(400, gin.H{"error": "invalid filter"})
				return where, args, false
			}
			where += " AND " + filter.column + "=?"
			args = append(args, value)
		}
	}
	return where, args, true
}

func (h *HTTP) workspaceFindings(c *gin.Context) {
	page, size := pagination(c)
	where, args := workspaceACL(currentUser(c))
	where, args, ok := workspaceFilters(c, where, args)
	if !ok {
		return
	}
	items, total, err := h.Store.workspaceFindings(c.Request.Context(), where, args, page, size)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page, "size": size})
}

func (s *Store) workspaceFindings(ctx context.Context, where string, args []any, page, size int) ([]WorkspaceFinding, int, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	from := ` FROM platform_runs JOIN platform_finding_index f ON f.run_id=platform_runs.id LEFT JOIN platform_reviews rv ON rv.run_id=f.run_id AND rv.finding_id=f.finding_id LEFT JOIN platform_finding_dispositions fd ON fd.run_id=f.run_id AND fd.finding_id=f.finding_id`
	total := 0
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*)"+from+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	params := append(append([]any{}, args...), size, (page-1)*size)
	// Extract only the selected finding summary. Never hydrate trace or evidence.
	path := `'$.findings['||f.ordinal||']'`
	query := `SELECT platform_runs.id,project_id,mr_iid,platform_runs.head_sha,platform_runs.status,f.finding_id,f.severity,f.kind,COALESCE(json_extract(result_json,` + path + `||'.title'),''),COALESCE(json_extract(result_json,` + path + `||'.file'),''),COALESCE(json_extract(result_json,` + path + `||'.line'),0),COALESCE(rv.status,'pending'),COALESCE(fd.status,'open'),COALESCE(fd.owner,0),COALESCE(fd.expires_at,'')` + from + where + ` ORDER BY platform_runs.id DESC,f.ordinal ASC LIMIT ? OFFSET ?`
	rows, err := tx.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, 0, err
	}
	items := []WorkspaceFinding{}
	for rows.Next() {
		var f WorkspaceFinding
		if err = rows.Scan(&f.RunID, &f.ProjectID, &f.MRIID, &f.HeadSHA, &f.RunStatus, &f.FindingID, &f.Severity, &f.Kind, &f.Title, &f.File, &f.Line, &f.ReviewStatus, &f.Disposition, &f.Owner, &f.ExpiresAt); err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (h *HTTP) workspaceTasks(c *gin.Context) {
	page, size := pagination(c)
	where, args := workspaceACL(currentUser(c))
	if value := c.Query("project_id"); value != "" {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			c.JSON(400, gin.H{"error": "invalid project_id"})
			return
		}
		where += " AND project_id=?"
		args = append(args, id)
	}
	switch c.Query("kind") {
	case "pending_review":
		where += ` AND EXISTS(SELECT 1 FROM platform_finding_index f LEFT JOIN platform_reviews rv ON rv.run_id=f.run_id AND rv.finding_id=f.finding_id WHERE f.run_id=platform_runs.id AND COALESCE(rv.status,'pending')='pending')`
	case "risk_expired":
		where += ` AND EXISTS(SELECT 1 FROM platform_finding_dispositions d JOIN platform_finding_index f ON f.run_id=d.run_id AND f.finding_id=d.finding_id WHERE d.run_id=platform_runs.id AND d.status='open' AND d.actor=0 AND d.revision>0)`
	case "failed":
		where += " AND status='failed'"
	case "incomplete":
		where += ` AND (status='incomplete' OR (status NOT IN ('pending','running') AND json_array_length(result_json,'$.coverage_notes')>0))`
	default:
		c.JSON(400, gin.H{"error": "invalid task kind"})
		return
	}
	items, total, err := h.Store.listRuns(c.Request.Context(), where, args, page, size)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page, "size": size, "kind": strings.TrimSpace(c.Query("kind"))})
}
