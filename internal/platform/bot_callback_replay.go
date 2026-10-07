package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"
)

func migrateBotCallbackReplay(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_bot_callback_receipts(integration_id INTEGER NOT NULL REFERENCES platform_integrations(id),request_hash TEXT NOT NULL,expires_at INTEGER NOT NULL,PRIMARY KEY(integration_id,request_hash)); CREATE INDEX IF NOT EXISTS platform_bot_callback_expiry ON platform_bot_callback_receipts(expires_at)`)
	return err
}

// Authenticate and consume the exact signed request once. This is an internal
// gate, not an action endpoint: user binding and project ACLs remain required.
func (s *Store) consumeSlackCallback(ctx context.Context, integrationID, revision int64, timestamp, signature string, body []byte, at time.Time) error {
	if integrationID <= 0 || revision <= 0 || len(body) > 65536 {
		return ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(credentials AS BLOB))<=65536 THEN credentials ELSE '' END FROM platform_integrations WHERE id=? AND revision=? AND kind='slack' AND enabled=1`, integrationID, revision).Scan(&raw); err != nil {
		return err
	}
	var c IntegrationCredentials
	if json.Unmarshal([]byte(raw), &c) != nil || !verifySlackCallback(c.Secret, timestamp, signature, body, at) {
		return ErrConflict
	}
	stamp, _ := strconv.ParseInt(timestamp, 10, 64)
	digest := sha256.Sum256(append([]byte("v0:"+timestamp+":"), body...))
	if _, err := tx.ExecContext(ctx, `DELETE FROM platform_bot_callback_receipts WHERE expires_at<?`, at.Unix()); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_bot_callback_receipts(integration_id,request_hash,expires_at) VALUES(?,?,?)`, integrationID, hex.EncodeToString(digest[:]), stamp+300)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
