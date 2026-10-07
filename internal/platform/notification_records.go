package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"github.com/gin-gonic/gin"
)

// Availability describes the current retry request gate, not a delivery promise.
type NotificationRecord struct {
	NotificationDelivery
	CanRetry               bool   `json:"can_retry"`
	RetryUnavailableReason string `json:"retry_unavailable_reason,omitempty"`
}

func notificationRetryUnavailable(d NotificationDelivery, revision sql.NullInt64, enabled sql.NullBool) string {
	if d.Status != "failed" && d.Status != "unknown" {
		return "state_not_retryable"
	}
	if d.Attempt < 1 {
		return "invalid_attempt"
	}
	if d.Attempt >= 5 {
		return "attempts_exhausted"
	}
	if !revision.Valid || revision.Int64 <= 0 || d.IntegrationRevision <= 0 || !enabled.Valid {
		return "configuration_unavailable"
	}
	if revision.Int64 != d.IntegrationRevision {
		return "configuration_changed"
	}
	if !enabled.Bool {
		return "integration_disabled"
	}
	return ""
}

func (s *Store) NotificationRecords(ctx context.Context, actor int64, page, size int) ([]NotificationRecord, int, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return nil, 0, err
	}
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_notification_deliveries`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.integration_id,d.integration_revision,d.project_id,d.run_id,d.event_key,d.status,d.attempt,d.next_attempt,d.error_code,d.created_at,d.updated_at,i.revision,i.enabled FROM platform_notification_deliveries d LEFT JOIN platform_integrations i ON i.id=d.integration_id ORDER BY d.id DESC LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	items := []NotificationRecord{}
	for rows.Next() {
		var d NotificationRecord
		var revision sql.NullInt64
		var enabled sql.NullBool
		if err = rows.Scan(&d.ID, &d.IntegrationID, &d.IntegrationRevision, &d.ProjectID, &d.RunID, &d.EventKey, &d.Status, &d.Attempt, &d.NextAttempt, &d.ErrorCode, &d.CreatedAt, &d.UpdatedAt, &revision, &enabled); err != nil {
			rows.Close()
			return nil, 0, err
		}
		d.RetryUnavailableReason = notificationRetryUnavailable(d.NotificationDelivery, revision, enabled)
		d.CanRetry = d.RetryUnavailableReason == ""
		items = append(items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	return items, total, tx.Commit()
}

type DeliveryRetryRequest struct {
	ExpectedAttempt      *int   `json:"expected_attempt"`
	ExpectedStatus       string `json:"expected_status"`
	AcknowledgeDuplicate bool   `json:"acknowledge_duplicate"`
}

func (s *Store) RetryNotification(ctx context.Context, id, actor int64, req DeliveryRetryRequest) error {
	if req.ExpectedAttempt == nil || *req.ExpectedAttempt < 1 || *req.ExpectedAttempt >= 5 {
		return ErrIntegrationInput
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return err
	}
	var status string
	var attempt int
	var integrationID, revision int64
	if err = tx.QueryRowContext(ctx, `SELECT status,attempt,integration_id,integration_revision FROM platform_notification_deliveries WHERE id=?`, id).Scan(&status, &attempt, &integrationID, &revision); err != nil {
		return err
	}
	if status != req.ExpectedStatus || attempt != *req.ExpectedAttempt {
		return ErrConflict
	}
	if status != "failed" && status != "unknown" {
		return ErrConflict
	}
	if status == "unknown" && !req.AcknowledgeDuplicate {
		return ErrIntegrationInput
	}
	var enabled bool
	var currentRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT enabled,revision FROM platform_integrations WHERE id=?`, integrationID).Scan(&enabled, &currentRevision); err != nil {
		return err
	}
	if !enabled || revision != currentRevision {
		return ErrConflict
	}
	stamp := now()
	if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='retry',next_attempt=?,error_code='',updated_at=? WHERE id=?`, stamp, stamp, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'notification.retry',?,?)`, actor, id, stamp); err != nil {
		return err
	}
	return tx.Commit()
}
func (h *HTTP) notificationRecords(c *gin.Context) {
	page, size := pagination(c)
	items, total, err := h.Store.NotificationRecords(c.Request.Context(), currentUser(c).ID, page, size)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page, "size": size})
}
func (h *HTTP) retryNotification(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req DeliveryRetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid retry request"})
		return
	}
	if err := h.Store.RetryNotification(c.Request.Context(), id, currentUser(c).ID, req); err != nil {
		if err == ErrIntegrationInput {
			c.JSON(400, gin.H{"error": "retry requires valid attempt and explicit duplicate acknowledgement for unknown result"})
			return
		}
		fail(c, err)
		return
	}
	c.Status(204)
}
func (h *HTTP) testIntegration(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	items, err := h.Store.Integrations(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		fail(c, err)
		return
	}
	var selected *Integration
	for i := range items {
		if items[i].ID == id {
			selected = &items[i]
		}
	}
	if selected == nil {
		fail(c, sql.ErrNoRows)
		return
	}
	switch selected.Kind {
	case "email", "feishu", "dingtalk", "wecom", "slack", "teams", "webhook":
	default:
		c.JSON(400, gin.H{"error": "notification test is unavailable for this integration type"})
		return
	}
	if !selected.Enabled || len(selected.ProjectIDs) == 0 {
		c.JSON(400, gin.H{"error": "enable and save the channel before testing"})
		return
	}
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		fail(c, err)
		return
	}
	deliveryID, err := h.Store.QueueNotification(c.Request.Context(), id, currentUser(c).ID, NotificationSummary{EventID: "test-" + hex.EncodeToString(raw), ProjectID: selected.ProjectIDs[0], Text: "AIMergeBot 通知渠道测试"})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, gin.H{"delivery_id": deliveryID, "status": "pending"})
}
