package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

type NotificationDelivery struct {
	ID                  int64               `json:"id"`
	IntegrationID       int64               `json:"integration_id"`
	IntegrationRevision int64               `json:"integration_revision"`
	ProjectID           int                 `json:"project_id"`
	RunID               int64               `json:"run_id"`
	EventKey            string              `json:"event_key"`
	Status              string              `json:"status"`
	Attempt             int                 `json:"attempt"`
	NextAttempt         string              `json:"next_attempt"`
	ErrorCode           string              `json:"error_code"`
	CreatedAt           string              `json:"created_at"`
	UpdatedAt           string              `json:"updated_at"`
	Payload             NotificationSummary `json:"-"`
	LeaseToken          string              `json:"-"`
}

func (s *Store) QueueNotification(ctx context.Context, integrationID, actor int64, summary NotificationSummary) (int64, error) {
	if summary.ProjectID <= 0 || len(summary.Text) > 6000 || len(summary.EventID) == 0 || len(summary.EventID) > 160 {
		return 0, ErrIntegrationInput
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return 0, err
	}
	integration, _, err := scanIntegration(tx.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, integrationID))
	if err != nil {
		return 0, err
	}
	allowed := false
	for _, id := range integration.ProjectIDs {
		if id == summary.ProjectID {
			allowed = true
		}
	}
	if !allowed || !integration.Enabled {
		return 0, ErrProjectPermission
	}
	if summary.RunID > 0 {
		snap, err := snapshotForRun(ctx, tx, summary.RunID)
		if err != nil {
			return 0, err
		}
		if snap.ProjectID != summary.ProjectID {
			return 0, ErrProjectPermission
		}
		if _, err = requireSnapshotRole(ctx, tx, snap, actor, "viewer"); err != nil {
			return 0, err
		}
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		return 0, err
	}
	stamp := now()
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_notification_deliveries(integration_id,integration_revision,project_id,run_id,event_key,next_attempt,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, integrationID, integration.Revision, summary.ProjectID, summary.RunID, summary.EventID, stamp, string(payload), stamp, stamp)
	if err != nil {
		return 0, err
	}
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_notification_deliveries WHERE integration_id=? AND event_key=?`, integrationID, summary.EventID).Scan(&id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Expired leases are unknown: a crashed sender may have reached the provider.
func (s *Store) ClaimNotification(ctx context.Context) (NotificationDelivery, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return NotificationDelivery{}, err
	}
	defer tx.Rollback()
	stamp := now()
	if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='unknown',error_code='sender_interrupted',lease_token='',lease_until='',updated_at=? WHERE status='sending' AND julianday(lease_until)<=julianday(?)`, stamp, stamp); err != nil {
		return NotificationDelivery{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='cancelled',error_code='configuration_changed',updated_at=? WHERE status IN ('pending','retry') AND NOT EXISTS(SELECT 1 FROM platform_integrations i WHERE i.id=integration_id AND i.revision=integration_revision AND i.enabled=1)`, stamp); err != nil {
		return NotificationDelivery{}, err
	}
	var d NotificationDelivery
	var payload string
	err = tx.QueryRowContext(ctx, `SELECT id,integration_id,integration_revision,project_id,run_id,event_key,status,attempt,next_attempt,error_code,payload,created_at,updated_at FROM platform_notification_deliveries WHERE status IN ('pending','retry') AND attempt<5 AND julianday(next_attempt)<=julianday(?) ORDER BY id LIMIT 1`, stamp).Scan(&d.ID, &d.IntegrationID, &d.IntegrationRevision, &d.ProjectID, &d.RunID, &d.EventKey, &d.Status, &d.Attempt, &d.NextAttempt, &d.ErrorCode, &payload, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		if commitErr := tx.Commit(); commitErr != nil {
			return d, commitErr
		}
		return d, err
	}
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal([]byte(payload), &d.Payload); err != nil {
		return d, err
	}
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		return d, err
	}
	d.LeaseToken = hex.EncodeToString(raw)
	lease := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='sending',attempt=attempt+1,lease_token=?,lease_until=?,updated_at=? WHERE id=? AND status IN ('pending','retry')`, d.LeaseToken, lease, stamp, d.ID)
	if err != nil {
		return d, err
	}
	d.Status = "sending"
	d.Attempt++
	return d, tx.Commit()
}
func (s *Store) FinishNotification(ctx context.Context, d NotificationDelivery, state, code string) error {
	if d.Attempt < 1 || d.Attempt > 5 || d.LeaseToken == "" {
		return ErrIntegrationInput
	}
	switch state {
	case "delivered", "accepted", "unknown", "failed", "retry", "cancelled":
	default:
		return ErrIntegrationInput
	}
	switch code {
	case "", "response_too_large", "rate_limited", "provider_server_error", "provider_http_rejected", "invalid_response", "missing_business_status", "provider_business_rejected", "unexpected_response", "unsupported_channel", "transport_unknown", "configuration_changed", "permission_changed", "invalid_configuration", "sender_interrupted":
	default:
		return ErrIntegrationInput
	}
	if state == "retry" && d.Attempt >= 5 {
		state = "failed"
	}
	next := time.Now().UTC().Add(time.Duration(1<<uint(d.Attempt)) * time.Minute).Format(time.RFC3339Nano)
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status=?,error_code=?,next_attempt=?,lease_token='',lease_until='',updated_at=? WHERE id=? AND status='sending' AND lease_token=? AND julianday(lease_until)>julianday(?) AND EXISTS(SELECT 1 FROM platform_integrations i WHERE i.id=integration_id AND i.revision=integration_revision)`, state, code, next, now(), d.ID, d.LeaseToken, now())
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
