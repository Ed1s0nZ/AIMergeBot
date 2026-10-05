package platform

import (
	"context"
	"database/sql"
)

// Database owns project membership. This durable outbox survives config-file failures.
func (s *Store) SyncProjectConfig(ctx context.Context, settings *SettingsService) error {
	s.projectSyncMu.Lock()
	defer s.projectSyncMu.Unlock()
	var generation int64
	var dirty bool
	if err := s.DB.QueryRowContext(ctx, `SELECT generation,dirty FROM platform_project_sync WHERE id=1`).Scan(&generation, &dirty); err != nil {
		return err
	}
	if !dirty {
		return nil
	}
	projects, err := s.Projects(ctx)
	if err == nil {
		for i := range projects {
			projects[i].ContextRepositories, err = s.ContextRepositories(ctx, projects[i].ID)
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = settings.SyncProjects(projects)
	}
	if err != nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE platform_project_sync SET last_error='configuration synchronization failed',updated_at=? WHERE id=1 AND generation=?`, now(), generation)
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE platform_project_sync SET dirty=0,last_error='',updated_at=? WHERE id=1 AND generation=?`, now(), generation)
	return err
}
func markProjectSync(tx *sql.Tx) error {
	_, err := tx.Exec(`UPDATE platform_project_sync SET generation=generation+1,dirty=1,updated_at=? WHERE id=1`, now())
	return err
}

type ProjectSyncStatus struct {
	Pending    bool   `json:"pending"`
	Generation int64  `json:"generation"`
	Error      string `json:"error"`
}

func (s *Store) ProjectSyncStatus(ctx context.Context) (ProjectSyncStatus, error) {
	var status ProjectSyncStatus
	err := s.DB.QueryRowContext(ctx, `SELECT dirty,generation,last_error FROM platform_project_sync WHERE id=1`).Scan(&status.Pending, &status.Generation, &status.Error)
	return status, err
}

// RestorePendingProjects prevents an old config file from overwriting a committed unsynced DB change.
func (s *Store) RestorePendingProjects(ctx context.Context, settings *SettingsService) (bool, error) {
	status, err := s.ProjectSyncStatus(ctx)
	if err != nil {
		return false, err
	}
	if !status.Pending {
		return true, nil
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
		return false, err
	}
	return true, nil
}
