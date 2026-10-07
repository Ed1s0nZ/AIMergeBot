package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	legacy "pr_agent/internal"
)

type ContextRepository = legacy.ContextRepositoryConfig

var ErrContextRepository = errors.New("invalid or unauthorized context repository")
var contextSHAPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func normalizeContextRepositories(project int, items []ContextRepository) ([]ContextRepository, error) {
	if len(items) > 4 {
		return nil, ErrContextRepository
	}
	out := append([]ContextRepository{}, items...)
	seen := map[int]bool{}
	for i := range out {
		item := &out[i]
		item.SHA = strings.ToLower(item.SHA)
		if item.ProjectID <= 0 || item.ProjectID == project || seen[item.ProjectID] || !contextSHAPattern.MatchString(item.SHA) {
			return nil, ErrContextRepository
		}
		seen[item.ProjectID] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectID < out[j].ProjectID })
	return out, nil
}

func migrateContextRepositories(tx *sql.Tx) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS platform_context_repositories(project_id INTEGER NOT NULL REFERENCES platform_projects(id),context_project_id INTEGER NOT NULL REFERENCES platform_projects(id),sha TEXT NOT NULL,PRIMARY KEY(project_id,context_project_id),CHECK(project_id<>context_project_id))`,
		`CREATE TABLE IF NOT EXISTS platform_run_context_repositories(run_id INTEGER NOT NULL REFERENCES platform_runs(id),project_id INTEGER NOT NULL,sha TEXT NOT NULL,PRIMARY KEY(run_id,project_id))`,
		`CREATE INDEX IF NOT EXISTS platform_context_runs ON platform_run_context_repositories(project_id,run_id)`,
		`CREATE TRIGGER IF NOT EXISTS platform_context_comment_isolation AFTER INSERT ON platform_comment_delivery WHEN EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=NEW.run_id) BEGIN UPDATE platform_comment_delivery SET state='blocked',last_error='Cross-repository report requires workspace permission intersection' WHERE run_id=NEW.run_id; END`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	var migrated bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM platform_access_migrations WHERE name='context-repositories-v1')`).Scan(&migrated); err != nil {
		return err
	}
	if !migrated {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO platform_run_context_repositories(run_id,project_id,sha) SELECT r.id,json_extract(c.value,'$.project_id'),json_extract(c.value,'$.sha') FROM platform_runs r,json_each(CASE WHEN json_valid(r.audit_policy_json) THEN r.audit_policy_json ELSE 'null' END,'$.context_repositories') c`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO platform_access_migrations(name) VALUES('context-repositories-v1')`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE platform_comment_delivery SET state='blocked',last_error='Cross-repository report requires workspace permission intersection' WHERE state IN ('pending','unknown') AND EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_comment_delivery.run_id)`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ContextRepositories(ctx context.Context, project int) ([]ContextRepository, error) {
	var id int
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&id); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT context_project_id,sha FROM platform_context_repositories WHERE project_id=? ORDER BY context_project_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContextRepository{}
	for rows.Next() {
		var item ContextRepository
		if err = rows.Scan(&item.ProjectID, &item.SHA); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SaveContextRepositories(ctx context.Context, project int, items []ContextRepository, actor int64) error {
	return s.saveContextRepositories(ctx, project, items, actor, false)
}
func (s *Store) saveContextRepositories(ctx context.Context, project int, items []ContextRepository, actor int64, importing bool) error {
	normalized, err := normalizeContextRepositories(project, items)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if importing {
		if err = requireLegacyRepositoryProject(ctx, tx, project); err != nil {
			return err
		}
	}
	if !importing {
		role, err := projectRole(ctx, tx, project, actor)
		if err != nil {
			return err
		}
		if role != "admin" {
			return ErrProjectPermission
		}
	}
	for _, id := range append([]int{project}, contextProjectIDs(normalized)...) {
		var enabled bool
		if err = tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, id).Scan(&enabled); err != nil {
			return err
		}
		if !importing && !enabled {
			return ErrContextRepository
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM platform_context_repositories WHERE project_id=?`, project); err != nil {
		return err
	}
	for _, item := range normalized {
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_context_repositories(project_id,context_project_id,sha) VALUES(?,?,?)`, project, item.ProjectID, item.SHA); err != nil {
			return err
		}
	}
	// An administrator replacing the allowed object set revokes unfinished readers
	// transactionally. Historical reports remain subject to their frozen ACL set.
	if _, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='context repository authorization changed',finished_at=? WHERE project_id=? AND status IN ('pending','running') AND EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id AND NOT EXISTS(SELECT 1 FROM platform_context_repositories a WHERE a.project_id=platform_runs.project_id AND a.context_project_id=c.project_id AND a.sha=c.sha))`, now(), project); err != nil {
		return err
	}
	if err = markProjectSync(tx); err != nil {
		return err
	}
	if !importing {
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'project.context.changed',?,?)`, actor, fmt.Sprint(project), now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func contextProjectIDs(items []ContextRepository) []int {
	out := []int{}
	for _, item := range items {
		out = append(out, item.ProjectID)
	}
	return out
}

// Startup config is trusted local administration. Existing DB outbox recovery
// must run first; nil on old configuration leaves existing links unchanged.
func (s *Store) ImportContextRepositories(ctx context.Context, projects []legacy.ProjectConfig) error {
	for _, project := range projects {
		if project.InternalProject || project.ContextRepositories == nil {
			continue
		}
		if err := s.saveContextRepositories(ctx, project.ID, project.ContextRepositories, 0, true); err != nil {
			return err
		}
	}
	return nil
}

func insertRunContextRepositories(ctx context.Context, tx *sql.Tx, id int64, policy *AuditPolicy) error {
	if policy == nil {
		return nil
	}
	for _, item := range policy.ContextRepositories {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_run_context_repositories(run_id,project_id,sha) VALUES(?,?,?)`, id, item.ProjectID, item.SHA); err != nil {
			return err
		}
	}
	return nil
}
