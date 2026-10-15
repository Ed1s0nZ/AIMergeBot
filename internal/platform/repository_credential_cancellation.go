package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

func snapshotUsesLegacyCredential(snap Snapshot, raw string) bool {
	if json.Unmarshal([]byte(raw), &snap.AuditPolicy) != nil || validateRepositoryScopeSnapshot(snap) != nil || snap.AuditPolicy == nil || snap.AuditPolicy.RepositoryScope == nil {
		return true
	}
	return scopeUsesLegacyCredential(snap.AuditPolicy.RepositoryScope)
}

func cancelLegacyCredentialRunsTx(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,project_id,source_project_id,mr_iid,diff_version_id,base_sha,head_sha,CASE WHEN length(CAST(audit_policy_json AS BLOB))<=65536 THEN audit_policy_json ELSE '' END FROM platform_runs WHERE status IN ('pending','running') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var snap Snapshot
		var raw string
		if err = rows.Scan(&id, &snap.ProjectID, &snap.SourceProjectID, &snap.MRIID, &snap.DiffVersionID, &snap.BaseSHA, &snap.HeadSHA, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if snapshotUsesLegacyCredential(snap, raw) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='repository credentials changed',finished_at=? WHERE id=? AND status IN ('pending','running')`, now(), id)
		if err != nil {
			return nil, err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return nil, errLegacyCredentialState
		}
	}
	return ids, nil
}
