package platform

import (
	"context"
	"database/sql"
	"strings"
)

func requireNotificationOwnerAccess(ctx context.Context, tx *sql.Tx, d NotificationDelivery) error {
	v, _, err := scanIntegration(tx.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, d.IntegrationID))
	if err != nil {
		return err
	}
	if v.Revision != d.IntegrationRevision || !v.Enabled {
		return ErrProjectPermission
	}
	// Runless explicit tests retain their administrator-authorized fixed payload.
	if d.RunID == 0 && strings.HasPrefix(d.EventKey, "test-") {
		return nil
	}
	if len(v.OwnerIDs) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT x.event_id,e.run_id FROM platform_notification_delivery_events x JOIN platform_notification_events e ON e.id=x.event_id WHERE x.delivery_id=? ORDER BY x.event_id LIMIT 201`, d.ID)
	if err != nil {
		return err
	}
	type source struct{ event, run int64 }
	items := []source{}
	for rows.Next() {
		var x source
		if err = rows.Scan(&x.event, &x.run); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var expected int
	if err = tx.QueryRowContext(ctx, `SELECT expected_events FROM platform_notification_delivery_event_counts WHERE delivery_id=?`, d.ID).Scan(&expected); err != nil {
		return err
	}
	if expected != len(items) {
		return ErrProjectPermission
	}
	if len(items) == 0 || len(items) > 200 {
		return ErrProjectPermission
	}
	for _, x := range items {
		if d.RunID > 0 && x.run != d.RunID {
			return ErrProjectPermission
		}
		if d.RunID == 0 {
			var exists bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_notification_delivery_runs WHERE delivery_id=? AND run_id=?)`, d.ID, x.run).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return ErrProjectPermission
			}
		}
		allowed, err := notificationOwnerEventAllowed(ctx, tx, v, x.event, d.ProjectID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrProjectPermission
		}
	}
	return nil
}
