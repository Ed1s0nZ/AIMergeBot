package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// RunListItem intentionally excludes evidence, diagrams, policy and tool traces.
type RunListItem struct {
	ID              int64      `json:"id"`
	ProjectID       int64      `json:"project_id"`
	SourceProjectID int64      `json:"source_project_id"`
	MRIID           int64      `json:"mr_iid"`
	BaseSHA         string     `json:"base_sha"`
	HeadSHA         string     `json:"head_sha"`
	Title           string     `json:"title"`
	URL             string     `json:"url"`
	Status          string     `json:"status"`
	Error           string     `json:"error"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	FindingCount    int        `json:"finding_count"`
	RetryAttempt    int        `json:"retry_attempt"`
	RetryParentID   int64      `json:"retry_parent_id,omitempty"`
	RetryChildID    int64      `json:"retry_child_id,omitempty"`
	RetryAt         string     `json:"retry_at,omitempty"`
	RetryInfo       *RetryInfo `json:"retry_info,omitempty"`
}

const runListColumns = `SELECT id,project_id,source_project_id,mr_iid,base_sha,head_sha,title,url,status,error,created_at,started_at,finished_at,
(SELECT COUNT(*) FROM platform_finding_index f WHERE f.run_id=platform_runs.id),retry_attempt,retry_parent_id,
COALESCE((SELECT child.id FROM platform_runs child WHERE child.retry_parent_id=platform_runs.id ORDER BY child.id DESC LIMIT 1),0),retry_at,retry_info_json FROM platform_runs`

// Count and rows share a read snapshot, including ACL and projection filters.
func (s *Store) listRuns(ctx context.Context, where string, args []any, page, size int) ([]RunListItem, int, error) {
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
	rows, err := tx.QueryContext(ctx, runListColumns+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, params...)
	if err != nil {
		return nil, 0, err
	}
	items := []RunListItem{}
	for rows.Next() {
		var r RunListItem
		var created, info string
		var started, finished sql.NullString
		err = rows.Scan(&r.ID, &r.ProjectID, &r.SourceProjectID, &r.MRIID, &r.BaseSHA, &r.HeadSHA, &r.Title, &r.URL, &r.Status, &r.Error, &created, &started, &finished, &r.FindingCount, &r.RetryAttempt, &r.RetryParentID, &r.RetryChildID, &r.RetryAt, &info)
		if err == nil {
			r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		}
		if err == nil && started.Valid {
			var t time.Time
			t, err = time.Parse(time.RFC3339Nano, started.String)
			r.StartedAt = &t
		}
		if err == nil && finished.Valid {
			var t time.Time
			t, err = time.Parse(time.RFC3339Nano, finished.String)
			r.FinishedAt = &t
		}
		if err == nil {
			err = json.Unmarshal([]byte(info), &r.RetryInfo)
		}
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		items = append(items, r)
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
