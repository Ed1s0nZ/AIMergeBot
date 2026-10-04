package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const workerLeaseDuration = 30 * time.Second
const workerHeartbeatInterval = 5 * time.Second

var ErrWorkerInstanceActive = errors.New("another worker instance is active; after a crash wait up to 30 seconds for its lease")
var ErrWorkerLeaseLost = errors.New("worker ownership lost")

func newWorkerOwner() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func migrateWorkerLease(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_worker_instance(id INTEGER PRIMARY KEY CHECK(id=1),owner TEXT NOT NULL,lease_until TEXT NOT NULL)`); err != nil {
		return err
	}
	rows, err := tx.Query(`PRAGMA table_info(platform_runs)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, required, pk int
		var name, kind string
		var fallback any
		if err = rows.Scan(&cid, &name, &kind, &required, &fallback, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, column := range []string{"worker_owner", "worker_lease_until"} {
		if !columns[column] {
			if _, err = tx.Exec(`ALTER TABLE platform_runs ADD COLUMN ` + column + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS platform_worker_owned_runs ON platform_runs(worker_owner,status)`)
	return err
}

func (s *Store) AcquireWorkerInstance(ctx context.Context, owner string) error {
	if owner == "" || len(owner) > 128 {
		return fmt.Errorf("invalid worker owner")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_worker_instance WHERE julianday(lease_until)>julianday('now'))`).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrWorkerInstanceActive
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_worker_instance(id,owner,lease_until) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner,lease_until=excluded.lease_until`, owner, time.Now().UTC().Add(workerLeaseDuration).Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func currentWorkerLease(ctx context.Context, tx *sql.Tx, owner string) (string, error) {
	if owner == "" {
		var active bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_worker_instance WHERE julianday(lease_until)>julianday('now'))`).Scan(&active)
		if err != nil {
			return "", err
		}
		if active {
			return "", ErrWorkerInstanceActive
		}
		return "", nil
	}
	var until string
	err := tx.QueryRowContext(ctx, `SELECT lease_until FROM platform_worker_instance WHERE id=1 AND owner=? AND julianday(lease_until)>julianday('now')`, owner).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrWorkerLeaseLost
	}
	return until, err
}

// RenewWorkerInstance cannot revive an expired instance or run lease.
func (s *Store) RenewWorkerInstance(ctx context.Context, owner string, activeIDs ...int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	until := time.Now().UTC().Add(workerLeaseDuration).Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `UPDATE platform_worker_instance SET lease_until=? WHERE id=1 AND owner=? AND julianday(lease_until)>julianday('now')`, until, owner)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrWorkerLeaseLost
	}
	if len(activeIDs) > 0 {
		placeholders := make([]string, len(activeIDs))
		args := []any{until, owner}
		for i, id := range activeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE platform_runs SET worker_lease_until=? WHERE worker_owner=? AND status='running' AND julianday(worker_lease_until)>julianday('now') AND id IN (`+strings.Join(placeholders, ",")+`)`, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) ReleaseWorkerInstance(ctx context.Context, owner string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE platform_worker_instance SET lease_until='' WHERE id=1 AND owner=?`, owner)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrWorkerLeaseLost
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_runs SET worker_lease_until='' WHERE worker_owner=? AND status='running'`, owner); err != nil {
		return err
	}
	return tx.Commit()
}

// All worker output writes require a live matching instance and run lease.
const workerFenceSQL = ` AND worker_owner=? AND ((worker_owner='' AND NOT EXISTS(SELECT 1 FROM platform_worker_instance i WHERE julianday(i.lease_until)>julianday('now'))) OR (worker_owner<>'' AND
 julianday(worker_lease_until)>julianday('now') AND EXISTS(SELECT 1 FROM platform_worker_instance i WHERE i.id=1 AND i.owner=platform_runs.worker_owner AND julianday(i.lease_until)>julianday('now'))))`

// Empty/malformed leases are expired; a live replacement instance also fences
// an old owner's rows even if their copied run deadline remains in the future.
const expiredWorkerSQL = ` AND (worker_owner='' OR COALESCE(julianday(worker_lease_until),0)<=julianday('now') OR NOT EXISTS(SELECT 1 FROM platform_worker_instance i WHERE i.id=1 AND i.owner=platform_runs.worker_owner AND julianday(i.lease_until)>julianday('now')))`

func (s *Store) OwnsRunningRun(ctx context.Context, id int64, owner string) error {
	var valid bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_runs WHERE id=? AND status='running'`+workerFenceSQL+`)`, id, owner).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrWorkerLeaseLost
	}
	return nil
}
func (s *Store) FailWorker(ctx context.Context, id int64, owner, message string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error=?,finished_at=? WHERE id=? AND status='running'`+workerFenceSQL, message, now(), id, owner)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrConflict
	}
	return nil
}

// Succeeded attempts can still be completing a comment request; keep their
// local context live while the same service owns the instance. Cancelled,
// recovered or expired attempts must stop even if a local cancellation missed.
func (s *Store) LiveWorkerContexts(ctx context.Context, owner string, ids []int64) (map[int64]bool, error) {
	valid := map[int64]bool{}
	if len(ids) == 0 {
		return valid, nil
	}
	placeholders := make([]string, len(ids))
	args := []any{owner, owner}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM platform_runs WHERE worker_owner=? AND EXISTS(SELECT 1 FROM platform_worker_instance WHERE id=1 AND owner=? AND julianday(lease_until)>julianday('now')) AND (status='succeeded' OR (status='running' AND julianday(worker_lease_until)>julianday('now'))) AND id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		valid[id] = true
	}
	return valid, rows.Err()
}

// ReleaseRunLease makes abandoned work recoverable without erasing its last
// durable progress. It cannot touch a replacement owner's run or revive a lease.
func (s *Store) ReleaseRunLease(ctx context.Context, id int64, owner string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET worker_lease_until='' WHERE id=? AND status='running'`+workerFenceSQL, id, owner)
	return err
}
