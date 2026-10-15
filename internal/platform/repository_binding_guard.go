package platform

import (
	"context"
)

// Until the factory supports frozen bindings, an explicit identity must never
// fall through to the legacy GitLab numeric-ID path. Even damaged rows block.
func requireLegacyRepositoryProject(ctx context.Context, q queryRower, project int) error {
	var bound bool
	if err := q.QueryRowContext(ctx, `WITH ids(id) AS (VALUES(?)) SELECT EXISTS(SELECT 1 FROM ids i WHERE `+legacyRepositoryUnavailablePredicate+`)`, project).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return ErrRepositoryUnavailable
	}
	return nil
}
func requireLegacyRepositorySnapshot(ctx context.Context, q queryRower, snap Snapshot) error {
	if snap.AuditPolicy != nil && snap.AuditPolicy.RepositoryScope != nil {
		return ErrRepositoryUnavailable
	}
	seen := map[int]bool{}
	ids := []int{snap.ProjectID, snap.SourceProjectID}
	for _, item := range contextPolicyItems(snap) {
		ids = append(ids, item.ProjectID)
	}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := requireLegacyRepositoryProject(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}
func requireLegacyRunRepository(ctx context.Context, q queryRower, run int64) error {
	var bound bool
	if err := q.QueryRowContext(ctx, `WITH ids(id) AS (SELECT project_id FROM platform_runs WHERE id=? UNION SELECT source_project_id FROM platform_runs WHERE id=? UNION SELECT project_id FROM platform_run_context_repositories WHERE run_id=?) SELECT EXISTS(SELECT 1 FROM ids i WHERE `+legacyRepositoryUnavailablePredicate+`) OR COALESCE((SELECT CASE WHEN json_valid(audit_policy_json) THEN EXISTS(SELECT 1 FROM json_each(audit_policy_json) WHERE lower(key)='repository_scope') ELSE 1 END FROM platform_runs WHERE id=?),0)`, run, run, run, run).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return ErrRepositoryUnavailable
	}
	return nil
}
func (s *Store) recordLegacyPollSeen(ctx context.Context, snap Snapshot) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = validateRepositoryCredentialsTx(ctx, s, tx, snap); err != nil {
		return err
	}
	if err = requireLegacyRepositorySnapshot(ctx, tx, snap); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_poll_seen(project_id,mr_iid,head_sha) VALUES(?,?,?) ON CONFLICT(project_id,mr_iid) DO UPDATE SET head_sha=excluded.head_sha`, snap.ProjectID, snap.MRIID, snap.HeadSHA); err != nil {
		return err
	}
	return tx.Commit()
}
