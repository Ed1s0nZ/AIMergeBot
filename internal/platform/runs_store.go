package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const PolicyVersion = "eino-sequence-v3"

var ErrConflict = errors.New("operation conflicts with current state")

// Enqueue serializes lookup and insertion so concurrent delivery cannot create duplicates.
func (s *Store) Enqueue(ctx context.Context, snap Snapshot, actor int64, force bool) (int64, bool, error) {
	if snap.ProjectID <= 0 || snap.MRIID <= 0 || snap.SourceProjectID <= 0 || snap.HeadSHA == "" || snap.BaseSHA == "" {
		return 0, false, fmt.Errorf("invalid snapshot")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var id int64
	q := `SELECT id FROM platform_runs WHERE project_id=? AND mr_iid=? AND head_sha=? AND policy_version=?`
	if force {
		q += ` AND status IN ('pending','running')`
	}
	q += ` ORDER BY id DESC LIMIT 1`
	err = tx.QueryRowContext(ctx, q, snap.ProjectID, snap.MRIID, snap.HeadSHA, PolicyVersion).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version) VALUES(?,?,?,?,?,?,?,?,'pending',?,?,?)`, snap.ProjectID, snap.MRIID, snap.SourceProjectID, snap.DiffVersionID, snap.BaseSHA, snap.HeadSHA, snap.Title, snap.URL, now(), actor, PolicyVersion)
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'run.created',?,?)`, actor, fmt.Sprint(id), now()); err != nil {
		return 0, false, err
	}
	return id, true, tx.Commit()
}

func (s *Store) Claim(ctx context.Context) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_runs WHERE status='pending' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='running',started_at=? WHERE id=? AND status='pending'`, now(), id)
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
	if status != "succeeded" && status != "failed" && status != "incomplete" && status != "cancelled" {
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
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET status=?,error=?,result_json=?,trace_json=?,finished_at=? WHERE id=? AND status='running'`, status, message, string(data), string(tr), now(), id)
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
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
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

func (s *Store) Recover(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error='interrupted by service restart; retry available',finished_at=? WHERE status='running'`, now())
	return err
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
