package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
)

func migrateDetailVersions(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_detail_versions(project_id INTEGER PRIMARY KEY,revision INTEGER NOT NULL DEFAULT 1)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO platform_detail_versions(project_id) VALUES(0)`); err != nil {
		return err
	}
	tables := []struct{ name, runColumn string }{
		{"platform_runs", "id"}, {"platform_reviews", "run_id"}, {"platform_comment_delivery", "run_id"},
		{"platform_review_history", "run_id"}, {"platform_finding_occurrences", "run_id"},
		{"platform_finding_association_history", "current_run"}, {"platform_run_context_repositories", "run_id"},
	}
	for _, table := range tables {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			row := "NEW"
			if event == "DELETE" {
				row = "OLD"
			}
			project := row + ".project_id"
			if table.name != "platform_runs" {
				project = "(SELECT project_id FROM platform_runs WHERE id=" + row + "." + table.runColumn + ")"
			}
			q := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS detail_%s_%s AFTER %s ON %s BEGIN INSERT INTO platform_detail_versions(project_id,revision) SELECT %s,1 WHERE %s IS NOT NULL ON CONFLICT(project_id) DO UPDATE SET revision=revision+1; END`, table.name, event, event, table.name, project, project)
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"platform_projects", "platform_project_members", "platform_users", "platform_context_repositories"} {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			q := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS detail_%s_%s AFTER %s ON %s BEGIN UPDATE platform_detail_versions SET revision=revision+1 WHERE project_id=0; END`, table, event, event, table)
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
	}
	return nil
}

type runDetailStatus struct {
	Version string     `json:"detail_version"`
	Status  string     `json:"status"`
	Wait    *QueueWait `json:"queue_wait"`
}

func (h *HTTP) detailStatus(ctx context.Context, id, actor int64, role string) (runDetailStatus, error) {
	var run Run
	var projectVersion, globalVersion int64
	err := h.Store.DB.QueryRowContext(ctx, `SELECT project_id,requested_by,status,retry_at,COALESCE((SELECT revision FROM platform_detail_versions WHERE project_id=platform_runs.project_id),0),(SELECT revision FROM platform_detail_versions WHERE project_id=0) FROM platform_runs WHERE id=?`, id).Scan(&run.ProjectID, &run.RequestedBy, &run.Status, &run.RetryAt, &projectVersion, &globalVersion)
	if err != nil {
		return runDetailStatus{}, err
	}
	wait, err := h.Store.QueueWaitFor(ctx, run)
	if err != nil {
		return runDetailStatus{}, err
	}
	enabled := h.Runner != nil && h.Runner.Settings != nil && h.Runner.Settings.Snapshot().EnableMRComment
	raw, _ := json.Marshal([]any{id, run.ProjectID, actor, role, projectVersion, globalVersion, enabled, wait})
	return runDetailStatus{Version: fmt.Sprintf("%x", sha256.Sum256(raw)), Status: run.Status, Wait: wait}, nil
}
