package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
)

type CheckDelivery struct {
	Actor    int64  `json:"actor"`
	RunID    int64  `json:"run_id"`
	HeadSHA  string `json:"head_sha"`
	State    string `json:"state"`
	Blocking bool   `json:"blocking"`
	RemoteID int    `json:"remote_id"`
	Code     string `json:"code"`
	Lease    string `json:"-"`
}

func migrateCheckDeliveries(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_check_deliveries(run_id INTEGER PRIMARY KEY REFERENCES platform_runs(id),head_sha TEXT NOT NULL,actor INTEGER NOT NULL,state TEXT NOT NULL DEFAULT 'pending',blocking INTEGER NOT NULL DEFAULT 0,remote_id INTEGER NOT NULL DEFAULT 0,code TEXT NOT NULL DEFAULT '',lease TEXT NOT NULL DEFAULT '',lease_until TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS platform_check_pending ON platform_check_deliveries(state,run_id)`)
	return err
}

func (s *Store) QueueRunCheck(ctx context.Context, runID, actor int64, blocking bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	snap, err := snapshotForRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	if _, err = requireSnapshotRole(ctx, tx, snap, actor, "admin"); err != nil {
		return err
	}
	var head, status string
	var newest int64
	if err = tx.QueryRowContext(ctx, `SELECT head_sha,status,(SELECT MAX(n.id) FROM platform_runs n WHERE n.project_id=r.project_id AND n.mr_iid=r.mr_iid) FROM platform_runs r WHERE r.id=?`, runID).Scan(&head, &status, &newest); err != nil {
		return err
	}
	if newest != runID || !commitID.MatchString(head) {
		return ErrConflict
	}
	switch status {
	case "succeeded", "failed", "incomplete", "cancelled", "skipped":
	default:
		return ErrConflict
	}
	var existingBlocking bool
	var existingHead string
	err = tx.QueryRowContext(ctx, `SELECT blocking,head_sha FROM platform_check_deliveries WHERE run_id=?`, runID).Scan(&existingBlocking, &existingHead)
	if err == nil {
		if existingBlocking != blocking || existingHead != head {
			return ErrConflict
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_check_deliveries(run_id,head_sha,actor,blocking,updated_at) VALUES(?,?,?,?,?)`, runID, head, actor, blocking, now()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'check.queued',?,?)`, actor, runID, now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClaimRunCheck(ctx context.Context) (CheckDelivery, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return CheckDelivery{}, err
	}
	defer tx.Rollback()
	stamp := now()
	if _, err = tx.ExecContext(ctx, `UPDATE platform_check_deliveries SET state='unknown',code='sender_interrupted',lease='',lease_until='',updated_at=? WHERE state='sending' AND julianday(lease_until)<=julianday(?)`, stamp, stamp); err != nil {
		return CheckDelivery{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_check_deliveries SET state='stale',code='newer_run',updated_at=? WHERE state='pending' AND EXISTS(SELECT 1 FROM platform_runs r JOIN platform_runs n ON n.project_id=r.project_id AND n.mr_iid=r.mr_iid AND n.id>r.id WHERE r.id=platform_check_deliveries.run_id)`, stamp); err != nil {
		return CheckDelivery{}, err
	}
	var d CheckDelivery
	err = tx.QueryRowContext(ctx, `SELECT run_id,head_sha,state,blocking,actor FROM platform_check_deliveries WHERE state='pending' ORDER BY run_id LIMIT 1`).Scan(&d.RunID, &d.HeadSHA, &d.State, &d.Blocking, &d.Actor)
	if err == sql.ErrNoRows {
		if e := tx.Commit(); e != nil {
			return d, e
		}
		return d, err
	}
	if err != nil {
		return d, err
	}
	token := make([]byte, 24)
	if _, err = rand.Read(token); err != nil {
		return d, err
	}
	d.Lease = hex.EncodeToString(token)
	d.State = "sending"
	_, err = tx.ExecContext(ctx, `UPDATE platform_check_deliveries SET state='sending',lease=?,lease_until=?,updated_at=? WHERE run_id=? AND state='pending'`, d.Lease, time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano), stamp, d.RunID)
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}

func (s *Store) FinishRunCheck(ctx context.Context, d CheckDelivery, result CheckPublication) error {
	switch result.State {
	case "published", "stale", "unknown", "failed":
	default:
		return ErrConflict
	}
	if result.State == "published" && result.RemoteID <= 0 {
		return ErrConflict
	}
	switch result.Code {
	case "", "invalid_identity", "invalid_target_url", "preflight_unavailable", "snapshot_changed", "publication_unacknowledged", "receipt_mismatch", "permission_changed":
	default:
		return ErrConflict
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_check_deliveries SET state=?,remote_id=?,code=?,lease='',lease_until='',updated_at=? WHERE run_id=? AND head_sha=? AND state='sending' AND lease=? AND lease!='' AND julianday(lease_until)>julianday(?)`, result.State, result.RemoteID, result.Code, now(), d.RunID, d.HeadSHA, d.Lease, now())
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) checkDeliveryAssessment(ctx context.Context, a CheckAssessment) (CheckAssessment, error) {
	var head, state string
	var blocking bool
	var remote int
	err := s.DB.QueryRowContext(ctx, `SELECT head_sha,state,blocking,remote_id FROM platform_check_deliveries WHERE run_id=?`, a.RunID).Scan(&head, &state, &blocking, &remote)
	if err == sql.ErrNoRows {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	a.PublicationState = state
	if head != a.HeadSHA {
		a.PublicationState = "unknown"
		return a, nil
	}
	if state == "published" && remote > 0 {
		a.Published = true
		a.Blocking = blocking
		a.RemoteID = remote
	}
	return a, nil
}
