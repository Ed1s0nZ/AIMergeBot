package platform

import (
	"context"
	"database/sql"
)

type BotBinding struct {
	IntegrationID  int64  `json:"integration_id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	WorkspaceID    string `json:"workspace_id"`
	ExternalUserID string `json:"external_user_id"`
	CreatedAt      string `json:"created_at"`
}

// Own identities remain visible for revocation after the channel is disabled.
// No channel credentials or other platform users are exposed.
func (s *Store) OwnBotBindings(ctx context.Context, actor int64) ([]BotBinding, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_users WHERE id=? AND disabled=0)`, actor).Scan(&active); err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrCredentials
	}
	rows, err := tx.QueryContext(ctx, `SELECT b.integration_id,i.name,i.enabled,b.workspace_id,b.external_user_id,b.created_at FROM platform_bot_bindings b JOIN platform_integrations i ON i.id=b.integration_id WHERE b.user_id=? ORDER BY b.integration_id,b.workspace_id LIMIT 501`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BotBinding{}
	for rows.Next() {
		if len(out) >= 500 {
			return nil, ErrConflict
		}
		var v BotBinding
		if err := rows.Scan(&v.IntegrationID, &v.Name, &v.Enabled, &v.WorkspaceID, &v.ExternalUserID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
