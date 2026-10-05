package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const PolicyVersion = "eino-audit-contract-v12"

var ErrConflict = errors.New("operation conflicts with current state")

// Enqueue is a trusted internal/system primitive. User submissions must use
// EnqueueUser. Lookup and insertion are serialized to prevent duplicate delivery.
func (s *Store) Enqueue(ctx context.Context, snap Snapshot, actor int64, force bool) (int64, bool, error) {
	return s.enqueue(ctx, snap, actor, force, false)
}

func (s *Store) EnqueueUser(ctx context.Context, snap Snapshot, actor int64, force bool) (int64, bool, error) {
	return s.enqueue(ctx, snap, actor, force, true)
}

func (s *Store) enqueue(ctx context.Context, snap Snapshot, actor int64, force, authorize bool) (int64, bool, error) {
	if snap.ProjectID <= 0 || snap.MRIID <= 0 || snap.SourceProjectID <= 0 || snap.HeadSHA == "" || snap.BaseSHA == "" {
		return 0, false, fmt.Errorf("invalid snapshot")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	if err = validateContextAdmission(ctx, tx, snap); err != nil {
		return 0, false, err
	}
	if authorize {
		if _, err = requireSnapshotRole(ctx, tx, snap, actor, "operator"); err != nil {
			return 0, false, err
		}
	}
	if err = validateFollowupEnqueue(ctx, tx, snap); err != nil {
		return 0, false, err
	}
	var id int64
	q := `SELECT id FROM platform_runs WHERE project_id=? AND mr_iid=? AND base_sha=? AND head_sha=? AND policy_version=? AND policy_digest=?`
	if force {
		q += ` AND status IN ('pending','running')`
	} else {
		q += ` AND status IN ('pending','running','succeeded','skipped')`
	}
	q += ` ORDER BY id DESC LIMIT 1`
	err = tx.QueryRowContext(ctx, q, snap.ProjectID, snap.MRIID, snap.BaseSHA, snap.HeadSHA, PolicyVersion, policyDigest(snap.AuditPolicy)).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if err = checkOutstanding(ctx, tx, snap.ProjectID, actor, s.auditQuotas()); err != nil {
		return 0, false, err
	}
	policyJSON, _ := json.Marshal(snap.AuditPolicy)
	res, err := tx.ExecContext(ctx, `INSERT INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version,policy_digest,audit_policy_json) VALUES(?,?,?,?,?,?,?,?,'pending',?,?,?,?,?)`, snap.ProjectID, snap.MRIID, snap.SourceProjectID, snap.DiffVersionID, snap.BaseSHA, snap.HeadSHA, snap.Title, snap.URL, now(), actor, PolicyVersion, policyDigest(snap.AuditPolicy), string(policyJSON))
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if err = insertRunContextRepositories(ctx, tx, id, snap.AuditPolicy); err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'run.created',?,?)`, actor, fmt.Sprint(id), now()); err != nil {
		return 0, false, err
	}
	return id, true, tx.Commit()
}

func (s *Store) Claim(ctx context.Context) (int64, error) { return s.claimRun(ctx, "") }
func (s *Store) ClaimOwned(ctx context.Context, owner string) (int64, error) {
	if owner == "" {
		return 0, ErrWorkerLeaseLost
	}
	return s.claimRun(ctx, owner)
}
func (s *Store) claimRun(ctx context.Context, owner string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	until, err := currentWorkerLease(ctx, tx, owner)
	if err != nil {
		return 0, err
	}
	var id int64
	predicate, args := quotaClaimPredicate(s.auditQuotas())
	if err = tx.QueryRowContext(ctx, `SELECT r.id FROM platform_runs r WHERE r.status='pending' AND (r.retry_at='' OR julianday(r.retry_at)<=julianday('now'))`+predicate+` ORDER BY r.id LIMIT 1`, args...).Scan(&id); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='running',started_at=?,worker_owner=?,worker_lease_until=? WHERE id=? AND status='pending'`, now(), owner, until, id)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, ErrConflict
	}
	return id, tx.Commit()
}

func (s *Store) Finish(ctx context.Context, id int64, status, message string, result AuditResult, trace []ToolTrace) error {
	return s.FinishOwned(ctx, id, "", status, message, result, trace)
}
func (s *Store) FinishOwned(ctx context.Context, id int64, owner, status, message string, result AuditResult, trace []ToolTrace) error {
	if status != "succeeded" && status != "failed" && status != "incomplete" && status != "cancelled" && status != "skipped" {
		return fmt.Errorf("invalid terminal state")
	}
	if result.Findings == nil {
		result.Findings = []Finding{}
	}
	if result.CoverageNotes == nil {
		result.CoverageNotes = []string{}
	}
	if trace == nil {
		trace = []ToolTrace{}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tr, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET status=?,error=?,result_json=?,trace_json=?,finished_at=? WHERE id=? AND status='running'`+workerFenceSQL, status, message, string(data), string(tr), now(), id, owner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) Cancel(ctx context.Context, id, actor int64) error {
	return s.cancelRun(ctx, id, actor, false)
}

func (s *Store) CancelUser(ctx context.Context, id, actor int64) error {
	return s.cancelRun(ctx, id, actor, true)
}

func (s *Store) cancelRun(ctx context.Context, id, actor int64, authorize bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if authorize {
		snap, e := snapshotForRun(ctx, tx, id)
		if e != nil {
			return e
		}
		if _, err = requireSnapshotRole(ctx, tx, snap, actor, "operator"); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='cancelled by user',finished_at=? WHERE id=? AND status IN ('pending','running')`, now(), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'run.cancelled',?,?)`, actor, fmt.Sprint(id), now()); err != nil {
		return err
	}
	return tx.Commit()
}

// Recover touches only orphaned/expired attempts; recovery itself rechecks the
// same predicate transactionally, preserving checkpoints without reserializing.
func (s *Store) Recover(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM platform_runs WHERE status='running'`+expiredWorkerSQL+` ORDER BY id LIMIT 200`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = s.recoverAttempt(ctx, id); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return nil
}

func (s *Store) Reviews(ctx context.Context, id int64) ([]Review, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT run_id,finding_id,status,reason,actor,updated_at FROM platform_reviews WHERE run_id=? ORDER BY finding_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Review{}
	for rows.Next() {
		var r Review
		if err = rows.Scan(&r.RunID, &r.FindingID, &r.Status, &r.Reason, &r.Actor, &r.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

func (s *Store) SaveReview(ctx context.Context, r Review) error {
	return s.saveReview(ctx, r, false)
}

func (s *Store) SaveReviewUser(ctx context.Context, r Review) error {
	return s.saveReview(ctx, r, true)
}

func (s *Store) saveReview(ctx context.Context, r Review, authorize bool) error {
	switch r.Status {
	case "pending", "accepted", "false_positive", "fixed":
	default:
		return fmt.Errorf("invalid review state")
	}
	if len(r.Reason) > 4000 {
		return fmt.Errorf("reason too long")
	}
	run, err := s.Run(ctx, r.RunID)
	if err != nil {
		return err
	}
	found := false
	for _, f := range run.Result.Findings {
		if f.ID == r.FindingID {
			found = true
			break
		}
	}
	if !found {
		return sql.ErrNoRows
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if authorize {
		if _, err = requireSnapshotRole(ctx, tx, run.Snapshot, r.Actor, "reviewer"); err != nil {
			return err
		}
	}
	var current int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_finding_index WHERE run_id=? AND finding_id=?`, r.RunID, r.FindingID).Scan(&current); err != nil {
		return err
	}
	if current == 0 {
		return sql.ErrNoRows
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_reviews(run_id,finding_id,status,reason,actor,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(run_id,finding_id) DO UPDATE SET status=excluded.status,reason=excluded.reason,actor=excluded.actor,updated_at=excluded.updated_at`, r.RunID, r.FindingID, r.Status, r.Reason, r.Actor, now())
	if err != nil {
		return err
	}
	detail, _ := json.Marshal(r)
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'finding.reviewed',?,?)`, r.Actor, string(detail), now()); err != nil {
		return err
	}
	return tx.Commit()
}
