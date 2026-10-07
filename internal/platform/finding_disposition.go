package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrDispositionInput = errors.New("invalid finding disposition")

type FindingDisposition struct {
	RunID            int64  `json:"run_id"`
	FindingID        string `json:"finding_id"`
	Revision         int64  `json:"revision"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	Owner            int64  `json:"owner"`
	ExpiresAt        string `json:"expires_at"`
	HeadSHA          string `json:"head_sha"`
	Actor            int64  `json:"actor"`
	UpdatedAt        string `json:"updated_at"`
	ManualResolution bool   `json:"manual_resolution"`
}
type DispositionRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	Owner            int64  `json:"owner"`
	ExpiresAt        string `json:"expires_at"`
	HeadSHA          string `json:"head_sha"`
	ManualResolution bool   `json:"manual_resolution"`
}
type DispositionReport struct {
	CanManage        bool                 `json:"can_manage"`
	Current          FindingDisposition   `json:"current"`
	History          []FindingDisposition `json:"history"`
	HistoryTruncated bool                 `json:"history_truncated"`
}

func migrateDispositions(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_finding_dispositions(run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,revision INTEGER NOT NULL,status TEXT NOT NULL,reason TEXT NOT NULL,owner INTEGER NOT NULL,expires_at TEXT NOT NULL,head_sha TEXT NOT NULL,actor INTEGER NOT NULL,updated_at TEXT NOT NULL,manual_resolution INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(run_id,finding_id))`,
		`CREATE TABLE IF NOT EXISTS platform_disposition_history(id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,revision INTEGER NOT NULL,status TEXT NOT NULL,reason TEXT NOT NULL,owner INTEGER NOT NULL,expires_at TEXT NOT NULL,head_sha TEXT NOT NULL,actor INTEGER NOT NULL,updated_at TEXT NOT NULL,manual_resolution INTEGER NOT NULL DEFAULT 0,UNIQUE(run_id,finding_id,revision))`,
		`CREATE INDEX IF NOT EXISTS platform_disposition_expiration ON platform_finding_dispositions(status,expires_at)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const dispositionColumns = `run_id,finding_id,revision,status,reason,owner,expires_at,head_sha,actor,updated_at,manual_resolution`

func scanDisposition(row interface{ Scan(...any) error }) (FindingDisposition, error) {
	var d FindingDisposition
	err := row.Scan(&d.RunID, &d.FindingID, &d.Revision, &d.Status, &d.Reason, &d.Owner, &d.ExpiresAt, &d.HeadSHA, &d.Actor, &d.UpdatedAt, &d.ManualResolution)
	return d, err
}
func requireDispositionFinding(ctx context.Context, tx *sql.Tx, runID, actor int64, findingID, required string) (Snapshot, error) {
	snap, err := snapshotForRun(ctx, tx, runID)
	if err != nil {
		return snap, err
	}
	if _, err = requireSnapshotRole(ctx, tx, snap, actor, required); err != nil {
		return snap, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT base_sha,head_sha,mr_iid FROM platform_runs WHERE id=?`, runID).Scan(&snap.BaseSHA, &snap.HeadSHA, &snap.MRIID); err != nil {
		return snap, err
	}
	var found int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM platform_finding_index WHERE run_id=? AND finding_id=? LIMIT 1`, runID, findingID).Scan(&found); err != nil {
		return snap, err
	}
	return snap, nil
}
func (s *Store) Disposition(ctx context.Context, runID, actor int64, findingID string) (DispositionReport, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DispositionReport{}, err
	}
	defer tx.Rollback()
	snap, err := requireDispositionFinding(ctx, tx, runID, actor, findingID, "viewer")
	if err != nil {
		return DispositionReport{}, err
	}
	d, err := scanDisposition(tx.QueryRowContext(ctx, `SELECT `+dispositionColumns+` FROM platform_finding_dispositions WHERE run_id=? AND finding_id=?`, runID, findingID))
	if err == sql.ErrNoRows {
		d = FindingDisposition{RunID: runID, FindingID: findingID, Status: "open", HeadSHA: snap.HeadSHA}
	} else if err != nil {
		return DispositionReport{}, err
	}
	role, err := requireSnapshotRole(ctx, tx, snap, actor, "viewer")
	if err != nil {
		return DispositionReport{}, err
	}
	report := DispositionReport{Current: d, History: []FindingDisposition{}, CanManage: roleRank(role) >= 3}
	rows, err := tx.QueryContext(ctx, `SELECT `+dispositionColumns+` FROM platform_disposition_history WHERE run_id=? AND finding_id=? ORDER BY revision DESC LIMIT 21`, runID, findingID)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		item, err := scanDisposition(rows)
		if err != nil {
			rows.Close()
			return report, err
		}
		report.History = append(report.History, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, err
	}
	if len(report.History) > 20 {
		report.History = report.History[:20]
		report.HistoryTruncated = true
	}
	return report, tx.Commit()
}
func (s *Store) SaveDisposition(ctx context.Context, runID, actor int64, findingID string, req DispositionRequest) (FindingDisposition, error) {
	if req.ExpectedRevision == nil || *req.ExpectedRevision < 0 || !utf8.ValidString(req.Reason) || len(req.Reason) > 4000 || req.Owner < 0 {
		return FindingDisposition{}, ErrDispositionInput
	}
	switch req.Status {
	case "open", "accepted", "resolved", "unknown":
	default:
		return FindingDisposition{}, ErrDispositionInput
	}
	if req.Status != "open" && strings.TrimSpace(req.Reason) == "" {
		return FindingDisposition{}, ErrDispositionInput
	}
	if req.Status == "resolved" && !req.ManualResolution {
		return FindingDisposition{}, ErrDispositionInput
	}
	if req.Status != "resolved" && req.ManualResolution {
		return FindingDisposition{}, ErrDispositionInput
	}
	if req.Status == "accepted" {
		expiry, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil || !expiry.After(time.Now()) || req.Owner <= 0 {
			return FindingDisposition{}, ErrDispositionInput
		}
		req.ExpiresAt = expiry.UTC().Format(time.RFC3339)
	} else {
		req.ExpiresAt = ""
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return FindingDisposition{}, err
	}
	defer tx.Rollback()
	snap, err := requireDispositionFinding(ctx, tx, runID, actor, findingID, "operator")
	if err != nil {
		return FindingDisposition{}, err
	}
	if req.HeadSHA != snap.HeadSHA || !commitID.MatchString(snap.HeadSHA) {
		return FindingDisposition{}, ErrConflict
	}
	if req.Owner > 0 {
		if _, err = requireSnapshotRole(ctx, tx, snap, req.Owner, "viewer"); err != nil {
			return FindingDisposition{}, ErrProjectPermission
		}
	}
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM platform_finding_dispositions WHERE run_id=? AND finding_id=?`, runID, findingID).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return FindingDisposition{}, err
	}
	if current != *req.ExpectedRevision {
		return FindingDisposition{}, ErrConflict
	}
	d := FindingDisposition{RunID: runID, FindingID: findingID, Revision: current + 1, Status: req.Status, Reason: req.Reason, Owner: req.Owner, ExpiresAt: req.ExpiresAt, HeadSHA: snap.HeadSHA, Actor: actor, UpdatedAt: now(), ManualResolution: req.ManualResolution}
	if err = writeDisposition(ctx, tx, d); err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func writeDisposition(ctx context.Context, tx *sql.Tx, d FindingDisposition) error {
	args := []any{d.RunID, d.FindingID, d.Revision, d.Status, d.Reason, d.Owner, d.ExpiresAt, d.HeadSHA, d.Actor, d.UpdatedAt, d.ManualResolution}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_finding_dispositions(`+dispositionColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(run_id,finding_id) DO UPDATE SET revision=excluded.revision,status=excluded.status,reason=excluded.reason,owner=excluded.owner,expires_at=excluded.expires_at,head_sha=excluded.head_sha,actor=excluded.actor,updated_at=excluded.updated_at,manual_resolution=excluded.manual_resolution`, args...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_disposition_history(`+dispositionColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, args...); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'finding.disposition',?,?)`, d.Actor, d.RunID, d.UpdatedAt)
	return err
}
func (s *Store) ExpireDispositions(ctx context.Context, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+dispositionColumns+` FROM platform_finding_dispositions WHERE status='accepted' AND julianday(expires_at)<=julianday(?) ORDER BY expires_at LIMIT 100`, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	items := []FindingDisposition{}
	for rows.Next() {
		d, err := scanDisposition(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, d := range items {
		d.Revision++
		d.Status = "open"
		d.Reason = "风险接受已到期，请重新评估。"
		d.ExpiresAt = ""
		d.ManualResolution = false
		d.Actor = 0
		d.UpdatedAt = at.UTC().Format(time.RFC3339Nano)
		if err = writeDisposition(ctx, tx, d); err != nil {
			return err
		}
		if err = recordDispositionNotificationEvent(ctx, tx, d, "risk.expired", fmt.Sprintf("expiry:%d:%s:%d", d.RunID, d.FindingID, d.Revision)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
