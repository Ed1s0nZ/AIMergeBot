package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

func migrateBotBindingChallenges(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_bot_binding_challenges(integration_id INTEGER NOT NULL REFERENCES platform_integrations(id),user_id INTEGER NOT NULL REFERENCES platform_users(id),integration_revision INTEGER NOT NULL,token_hash TEXT NOT NULL UNIQUE,expires_at INTEGER NOT NULL,PRIMARY KEY(integration_id,user_id))`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS platform_bot_bindings(integration_id INTEGER NOT NULL REFERENCES platform_integrations(id),workspace_id TEXT NOT NULL,external_user_id TEXT NOT NULL,user_id INTEGER NOT NULL REFERENCES platform_users(id),created_at TEXT NOT NULL,PRIMARY KEY(integration_id,workspace_id,external_user_id),UNIQUE(integration_id,workspace_id,user_id))`)
	return err
}

type botBindingChallenge struct {
	Token     string
	ExpiresAt time.Time
}

// Called only for an authenticated platform user initiating their own binding.
// It grants no project permissions and cannot name an external user directly.
func (s *Store) issueSlackBindingChallenge(ctx context.Context, actor, integrationID, revision int64, at time.Time) (botBindingChallenge, error) {
	if actor <= 0 || integrationID <= 0 || revision <= 0 {
		return botBindingChallenge{}, ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return botBindingChallenge{}, err
	}
	defer tx.Rollback()
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_users WHERE id=? AND disabled=0)`, actor).Scan(&active); err != nil {
		return botBindingChallenge{}, err
	}
	if !active {
		return botBindingChallenge{}, ErrCredentials
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(credentials AS BLOB))<=65536 THEN credentials ELSE '' END FROM platform_integrations WHERE id=? AND revision=? AND kind='slack' AND enabled=1`, integrationID, revision).Scan(&raw); err != nil {
		return botBindingChallenge{}, err
	}
	var credentials IntegrationCredentials
	if json.Unmarshal([]byte(raw), &credentials) != nil || credentials.Secret == "" || len(credentials.Secret) > 4096 {
		return botBindingChallenge{}, ErrConflict
	}
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		return botBindingChallenge{}, err
	}
	token := hex.EncodeToString(seed)
	digest := sha256.Sum256([]byte(token))
	expires := at.Add(10 * time.Minute)
	if _, err := tx.ExecContext(ctx, `DELETE FROM platform_bot_binding_challenges WHERE expires_at<=?`, at.Unix()); err != nil {
		return botBindingChallenge{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_bot_binding_challenges(integration_id,user_id,integration_revision,token_hash,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(integration_id,user_id) DO UPDATE SET integration_revision=excluded.integration_revision,token_hash=excluded.token_hash,expires_at=excluded.expires_at`, integrationID, actor, revision, hex.EncodeToString(digest[:]), expires.Unix()); err != nil {
		return botBindingChallenge{}, err
	}
	if err := tx.Commit(); err != nil {
		return botBindingChallenge{}, err
	}
	return botBindingChallenge{Token: token, ExpiresAt: expires}, nil
}
