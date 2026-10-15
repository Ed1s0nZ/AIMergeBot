package platform

import "context"

// This consistency check is not an ACL check or permission to execute a scope.
func validateRepositoryScopeContextIndex(ctx context.Context, q queryRower, run int64, snap Snapshot) error {
	if snap.AuditPolicy == nil || snap.AuditPolicy.RepositoryScope == nil {
		return nil
	}
	if run <= 0 || validateRepositoryScopeSnapshot(snap) != nil {
		return ErrRepositoryScope
	}
	items := snap.AuditPolicy.RepositoryScope.Context
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_run_context_repositories WHERE run_id=?`, run).Scan(&count); err != nil {
		return err
	}
	if count != len(items) {
		return ErrRepositoryScope
	}
	for _, item := range items {
		var matches bool
		if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_run_context_repositories WHERE run_id=? AND project_id=? AND sha=?)`, run, item.Repository.ProjectID, item.SHA).Scan(&matches); err != nil {
			return err
		}
		if !matches {
			return ErrRepositoryScope
		}
	}
	return nil
}
