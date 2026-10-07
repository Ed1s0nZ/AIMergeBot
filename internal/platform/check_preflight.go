package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Permission, latest-run identity and lease are observed in one read snapshot.
func (s *Store) CheckPublicationRun(ctx context.Context, d CheckDelivery) (Run, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	snap, err := snapshotForRun(ctx, tx, d.RunID)
	if err != nil {
		return Run{}, err
	}
	if _, err = requireSnapshotRole(ctx, tx, snap, d.Actor, "admin"); err != nil {
		return Run{}, err
	}
	if d.Automatic {
		if err = validateAutomaticCheckPolicy(ctx, tx, snap, d.Actor, d.Blocking); err != nil {
			return Run{}, err
		}
	}
	var original int64
	if err = tx.QueryRowContext(ctx, `SELECT requested_by FROM platform_runs WHERE id=?`, d.RunID).Scan(&original); err != nil {
		return Run{}, err
	}
	if original > 0 {
		if _, err = requireSnapshotRole(ctx, tx, snap, original, "viewer"); err != nil {
			return Run{}, err
		}
	}
	projects := []int{snap.ProjectID, snap.SourceProjectID}
	for _, c := range contextPolicyItems(snap) {
		projects = append(projects, c.ProjectID)
	}
	for _, p := range projects {
		var enabled bool
		if err = tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, p).Scan(&enabled); err != nil {
			return Run{}, err
		}
		if !enabled {
			return Run{}, ErrProjectPermission
		}
	}
	if err := requireLegacyRepositorySnapshot(ctx, tx, snap); err != nil {
		return Run{}, err
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_check_deliveries d JOIN platform_runs r ON r.id=d.run_id WHERE d.run_id=? AND d.actor=? AND d.automatic=? AND d.blocking=? AND d.head_sha=? AND r.head_sha=d.head_sha AND d.state='sending' AND d.lease=? AND d.lease!='' AND julianday(d.lease_until)>julianday(?) AND NOT EXISTS(SELECT 1 FROM platform_runs n WHERE n.project_id=r.project_id AND n.mr_iid=r.mr_iid AND n.id>r.id))`, d.RunID, d.Actor, d.Automatic, d.Blocking, d.HeadSHA, d.Lease, now()).Scan(&valid)
	if err != nil {
		return Run{}, err
	}
	if !valid {
		return Run{}, ErrConflict
	}
	run := Run{ID: d.RunID, Snapshot: snap, RequestedBy: original}
	var result string
	var size int
	err = tx.QueryRowContext(ctx, `SELECT base_sha,head_sha,mr_iid,status,error,length(CAST(result_json AS BLOB)),CASE WHEN length(CAST(result_json AS BLOB))<=1048576 THEN result_json ELSE '' END FROM platform_runs WHERE id=?`, d.RunID).Scan(&run.BaseSHA, &run.HeadSHA, &run.MRIID, &run.Status, &run.Error, &size, &result)
	if err != nil {
		return Run{}, err
	}
	if size > 1048576 {
		return Run{}, ErrConflict
	}
	if err = json.Unmarshal([]byte(result), &run.Result); err != nil {
		return Run{}, err
	}
	return run, tx.Commit()
}

func checkPreflightFailure(err error) CheckPublication {
	if errors.Is(err, ErrRepositoryUnavailable) {
		return CheckPublication{State: "failed", Code: "repository_unavailable"}
	}
	if errors.Is(err, ErrConflict) {
		return CheckPublication{State: "stale", Code: "snapshot_changed"}
	}
	if errors.Is(err, ErrProjectPermission) || errors.Is(err, ErrCredentials) || errors.Is(err, sql.ErrNoRows) {
		return CheckPublication{State: "failed", Code: "permission_changed"}
	}
	return CheckPublication{State: "failed", Code: "preflight_unavailable"}
}
