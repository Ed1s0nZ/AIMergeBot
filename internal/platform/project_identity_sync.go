package platform

import (
	"context"
	"database/sql"
	"errors"
	legacy "pr_agent/internal"
)

func saveProjectTx(ctx context.Context, tx *sql.Tx, p Project) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO platform_projects(id,name,enabled) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,enabled=excluded.enabled`, p.ID, p.Name, p.Enabled)
	if err != nil {
		return err
	}
	if !p.Enabled {
		if _, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='context repository authorization changed',finished_at=? WHERE status IN ('pending','running') AND (EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id AND c.project_id=?) OR (project_id=? AND EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id)))`, now(), p.ID, p.ID); err != nil {
			return err
		}
	}
	return markProjectSync(tx)
}

// SaveLegacyProject admits GitLab IDs without reinterpreting bound internal IDs.
func (s *Store) SaveLegacyProject(ctx context.Context, p Project) error {
	if p.ID <= 0 || len(p.Name) == 0 || len(p.Name) > 200 {
		return errors.New("invalid project")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireLegacyRepositoryProject(ctx, tx, p.ID); err != nil {
		if errors.Is(err, ErrRepositoryUnavailable) {
			return ErrConflict
		}
		return err
	}
	if err = saveProjectTx(ctx, tx, p); err != nil {
		return err
	}
	return tx.Commit()
}

// ImportConfiguredProjects returns only legacy entries eligible for context import.
// Internal references preserve the database's project and binding authority.
func (s *Store) ImportConfiguredProjects(ctx context.Context, projects []legacy.ProjectConfig) ([]legacy.ProjectConfig, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	seen := make(map[int]bool)
	legacyProjects := []legacy.ProjectConfig{}
	for _, p := range projects {
		if p.ID <= 0 || seen[p.ID] {
			return nil, ErrConflict
		}
		seen[p.ID] = true
		binding, err := readRepositoryBinding(ctx, tx, p.ID)
		if err != nil {
			return nil, err
		}
		if p.InternalProject {
			if binding.Revision == 0 {
				return nil, ErrConflict
			}
			var exists int
			if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, p.ID).Scan(&exists); err != nil {
				return nil, err
			}
			continue
		}
		if binding.Revision != 0 || len(p.Name) == 0 || len(p.Name) > 200 {
			return nil, ErrConflict
		}
		enabled := true
		err = tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, p.ID).Scan(&enabled)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if p.Enabled != nil {
			enabled = *p.Enabled
		}
		if err = saveProjectTx(ctx, tx, Project{ID: p.ID, Name: p.Name, Enabled: enabled}); err != nil {
			return nil, err
		}
		legacyProjects = append(legacyProjects, p)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return legacyProjects, nil
}

// projectConfiguration captures identity, context and the outbox generation
// together. File persistence happens after this short database transaction.
func (s *Store) projectConfiguration(ctx context.Context) ([]legacy.ProjectConfig, int64, bool, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, false, err
	}
	defer tx.Rollback()
	var generation int64
	var dirty bool
	if err = tx.QueryRowContext(ctx, `SELECT generation,dirty FROM platform_project_sync WHERE id=1`).Scan(&generation, &dirty); err != nil {
		return nil, 0, false, err
	}
	projects := []legacy.ProjectConfig{}
	if dirty {
		rows, err := tx.QueryContext(ctx, `SELECT id,name,enabled FROM platform_projects ORDER BY name,id`)
		if err != nil {
			return nil, 0, false, err
		}
		for rows.Next() {
			var p legacy.ProjectConfig
			var enabled bool
			if err = rows.Scan(&p.ID, &p.Name, &enabled); err != nil {
				rows.Close()
				return nil, 0, false, err
			}
			p.Enabled = &enabled
			projects = append(projects, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, 0, false, err
		}
		for i := range projects {
			p := &projects[i]
			binding, err := readRepositoryBinding(ctx, tx, p.ID)
			if err != nil {
				return nil, 0, false, err
			}
			p.InternalProject = binding.Revision != 0
			rows, err := tx.QueryContext(ctx, `SELECT context_project_id,sha FROM platform_context_repositories WHERE project_id=? ORDER BY context_project_id`, p.ID)
			if err != nil {
				return nil, 0, false, err
			}
			for rows.Next() {
				var item legacy.ContextRepositoryConfig
				if err = rows.Scan(&item.ProjectID, &item.SHA); err != nil {
					rows.Close()
					return nil, 0, false, err
				}
				p.ContextRepositories = append(p.ContextRepositories, item)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, 0, false, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, false, err
	}
	return projects, generation, dirty, nil
}
