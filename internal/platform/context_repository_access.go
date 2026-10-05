package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
)

func contextPolicyItems(snap Snapshot) []ContextRepository {
	if snap.AuditPolicy == nil {
		return nil
	}
	return snap.AuditPolicy.ContextRepositories
}
func validateContextAdmission(ctx context.Context, q queryRower, snap Snapshot) error {
	items := contextPolicyItems(snap)
	if len(items) == 0 {
		return nil
	}
	normalized, err := normalizeContextRepositories(snap.ProjectID, items)
	if err != nil || !reflect.DeepEqual(items, normalized) {
		return ErrContextRepository
	}
	for _, item := range items {
		var authorized bool
		if err = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_context_repositories c JOIN platform_projects p ON p.id=c.context_project_id JOIN platform_projects parent ON parent.id=c.project_id WHERE c.project_id=? AND c.context_project_id=? AND c.sha=? AND p.enabled=1 AND parent.enabled=1)`, snap.ProjectID, item.ProjectID, item.SHA).Scan(&authorized); err != nil {
			return err
		}
		if !authorized {
			return ErrContextRepository
		}
	}
	return nil
}
func requireContextRoles(ctx context.Context, q queryRower, snap Snapshot, user int64) error {
	items := contextPolicyItems(snap)
	if len(items) == 0 {
		return nil
	}
	if _, err := normalizeContextRepositories(snap.ProjectID, items); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := requireProjectRole(ctx, q, item.ProjectID, user, "viewer"); err != nil {
			return err
		}
	}
	return nil
}
func snapshotForRun(ctx context.Context, q queryRower, id int64) (Snapshot, error) {
	var snap Snapshot
	var policy string
	err := q.QueryRowContext(ctx, `SELECT project_id,source_project_id,audit_policy_json FROM platform_runs WHERE id=?`, id).Scan(&snap.ProjectID, &snap.SourceProjectID, &policy)
	if err != nil {
		return snap, err
	}
	err = json.Unmarshal([]byte(policy), &snap.AuditPolicy)
	return snap, err
}

const contextListACL = ` AND NOT EXISTS(SELECT 1 FROM platform_run_context_repositories ctx LEFT JOIN platform_project_members cm ON cm.project_id=ctx.project_id AND cm.user_id=? WHERE ctx.run_id=platform_runs.id AND cm.user_id IS NULL)`

func cancelRevokedContextMember(ctx context.Context, tx *sql.Tx, project int, user int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='project execution permission revoked',finished_at=? WHERE requested_by=? AND status IN ('pending','running') AND EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id AND c.project_id=?)`, now(), user, project)
	return err
}
