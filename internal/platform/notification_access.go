package platform

import (
	"context"
	"database/sql"
	"strings"
)

// Digest source identities stay server-side; recheck all in one read snapshot.
func (s *Store) requireNotificationAccess(ctx context.Context, d NotificationDelivery) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ids := []int64{}
	if d.RunID > 0 {
		ids = append(ids, d.RunID)
	} else if strings.HasPrefix(d.EventKey, "digest:") {
		rows, err := tx.QueryContext(ctx, `SELECT run_id FROM platform_notification_delivery_runs WHERE delivery_id=? ORDER BY run_id`, d.ID)
		if err != nil {
			return err
		}
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
		// Legacy digests cannot prove their source permission snapshot.
		if len(ids) == 0 {
			return sql.ErrNoRows
		}
	}
	for _, id := range ids {
		snap, err := snapshotForRun(ctx, tx, id)
		if err != nil {
			return err
		}
		if snap.ProjectID != d.ProjectID {
			return sql.ErrNoRows
		}
		var actor int64
		if err = tx.QueryRowContext(ctx, `SELECT requested_by FROM platform_runs WHERE id=?`, id).Scan(&actor); err != nil {
			return err
		}
		if _, err = requireSnapshotRole(ctx, tx, snap, actor, "viewer"); err != nil {
			return err
		}
	}
	if err = requireNotificationOwnerAccess(ctx, tx, d); err != nil {
		return err
	}
	return tx.Commit()
}
