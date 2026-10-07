package platform

import (
	"context"
	"database/sql"
	"errors"
)

func validateNotificationOwnerRoute(v Integration) error {
	if len(v.OwnerIDs) == 0 {
		return nil
	}
	if len(v.OwnerIDs) > 100 || len(v.Events) == 0 {
		return ErrIntegrationInput
	}
	switch v.Kind {
	case "email", "feishu", "dingtalk", "wecom", "slack", "teams", "webhook":
	default:
		return ErrIntegrationInput
	}
	for _, kind := range v.Events {
		if kind != "finding.reviewed" && kind != "risk.expired" {
			return ErrIntegrationInput
		}
	}
	seen := map[int64]bool{}
	for _, id := range v.OwnerIDs {
		if id <= 0 || seen[id] {
			return ErrIntegrationInput
		}
		seen[id] = true
	}
	return nil
}

func requireNotificationRouteUsers(ctx context.Context, tx *sql.Tx, v Integration) error {
	for _, id := range v.OwnerIDs {
		for _, project := range v.ProjectIDs {
			var enabled bool
			if err := tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, project).Scan(&enabled); err != nil {
				return err
			}
			if !enabled {
				return ErrProjectPermission
			}
			if _, err := requireProjectRole(ctx, tx, project, id, "viewer"); err != nil {
				return err
			}
		}
	}
	return nil
}

// The event's captured assignment must still be current; never retarget old events.
func notificationOwnerEventAllowed(ctx context.Context, tx *sql.Tx, v Integration, eventID int64, project int) (bool, error) {
	var run, owner, revision, currentOwner, currentRevision, actor int64
	var kind, head, currentHead, runHead string
	err := tx.QueryRowContext(ctx, `SELECT e.run_id,e.kind,f.owner,f.disposition_revision,f.head_sha,d.owner,d.revision,d.head_sha,r.head_sha,r.requested_by
FROM platform_notification_events e JOIN platform_notification_event_findings f ON f.event_id=e.id
JOIN platform_runs r ON r.id=e.run_id JOIN platform_finding_dispositions d ON d.run_id=e.run_id AND d.finding_id=f.finding_id WHERE e.id=?`, eventID).Scan(&run, &kind, &owner, &revision, &head, &currentOwner, &currentRevision, &currentHead, &runHead, &actor)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	selected := false
	for _, id := range v.OwnerIDs {
		if id == owner {
			selected = true
		}
	}
	if !selected || revision <= 0 || (kind != "finding.reviewed" && kind != "risk.expired") || owner != currentOwner || revision != currentRevision || head != currentHead || head != runHead || !validCodeOwnerSHA(head) {
		return false, nil
	}
	snap, err := snapshotForRun(ctx, tx, run)
	if err != nil {
		return false, err
	}
	if snap.ProjectID != project {
		return false, nil
	}
	if err = requireTicketProjectsEnabled(ctx, tx, snap); err != nil {
		return notificationOwnerPermissionResult(err)
	}
	for _, id := range []int64{actor, owner} {
		if _, err = requireSnapshotRole(ctx, tx, snap, id, "viewer"); err != nil {
			return notificationOwnerPermissionResult(err)
		}
	}
	return true, nil
}

func notificationOwnerPermissionResult(err error) (bool, error) {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrProjectPermission) || errors.Is(err, ErrCredentials) {
		return false, nil
	}
	return false, err
}

func recordDeliveryEvent(ctx context.Context, tx *sql.Tx, integrationID int64, key string, eventID int64) error {
	if eventID <= 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_delivery_events(delivery_id,event_id) SELECT id,? FROM platform_notification_deliveries WHERE integration_id=? AND event_key=?`, eventID, integrationID, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_notification_delivery_event_counts(delivery_id,expected_events)
SELECT id,1 FROM platform_notification_deliveries WHERE integration_id=? AND event_key=?
ON CONFLICT(delivery_id) DO UPDATE SET expected_events=expected_events+1`, integrationID, key)
	return err
}
