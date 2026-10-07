package platform

import "context"

// Revocation is scoped to the authenticated actor, never an external name.
// It remains available when the channel is disabled or reconfigured.
func (s *Store) revokeSlackBinding(ctx context.Context, actor, integrationID int64) (bool, error) {
	if actor <= 0 || integrationID <= 0 {
		return false, ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_users WHERE id=? AND disabled=0)`, actor).Scan(&active); err != nil {
		return false, err
	}
	if !active {
		return false, ErrCredentials
	}
	bindings, err := tx.ExecContext(ctx, `DELETE FROM platform_bot_bindings WHERE integration_id=? AND user_id=?`, integrationID, actor)
	if err != nil {
		return false, err
	}
	challenges, err := tx.ExecContext(ctx, `DELETE FROM platform_bot_binding_challenges WHERE integration_id=? AND user_id=?`, integrationID, actor)
	if err != nil {
		return false, err
	}
	n, err := bindings.RowsAffected()
	if err != nil {
		return false, err
	}
	m, err := challenges.RowsAffected()
	if err != nil {
		return false, err
	}
	if n+m > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'bot.binding.revoked',?,?)`, actor, integrationID, now()); err != nil {
			return false, err
		}
	}
	return n+m > 0, tx.Commit()
}
