package platform

import (
	"context"
	"database/sql"
)

// recordDispositionNotificationEvent commits with the disposition and its history.
// Legacy events are deliberately not backfilled from mutable current ownership.
func recordDispositionNotificationEvent(ctx context.Context, tx *sql.Tx, d FindingDisposition, kind, key string) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_events(run_id,kind,event_key,severity,created_at)
VALUES(?,?,?,COALESCE((SELECT CASE severity WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END FROM platform_finding_index WHERE run_id=? AND finding_id=? LIMIT 1),0),?)`, d.RunID, kind, key, d.RunID, d.FindingID, d.UpdatedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_event_findings(event_id,finding_id,disposition_revision,owner,head_sha)
SELECT id,?,?,?,? FROM platform_notification_events WHERE event_key=? AND run_id=? AND kind=?`, d.FindingID, d.Revision, d.Owner, d.HeadSHA, key, d.RunID, kind)
	return err
}
