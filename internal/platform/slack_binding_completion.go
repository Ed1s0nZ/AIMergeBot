package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var slackAppID = regexp.MustCompile(`^A[A-Z0-9]{1,63}$`)
var slackWorkspaceID = regexp.MustCompile(`^T[A-Z0-9]{1,63}$`)
var slackUserID = regexp.MustCompile(`^[UW][A-Z0-9]{1,63}$`)
var bindingToken = regexp.MustCompile(`^[0-9a-f]{48}$`)

func (s *Store) completeSlackBinding(ctx context.Context, integrationID, revision int64, timestamp, signature string, body []byte, at time.Time) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := consumeSlackCallbackTx(ctx, tx, integrationID, revision, timestamp, signature, body, at); err != nil {
		return 0, err
	}
	fields, err := url.ParseQuery(string(body))
	if err != nil {
		return 0, ErrConflict
	}
	for _, field := range []string{"team_id", "user_id", "text"} {
		if len(fields[field]) != 1 {
			return 0, ErrConflict
		}
	}
	workspace, external, text := fields.Get("team_id"), fields.Get("user_id"), fields.Get("text")
	if !slackWorkspaceID.MatchString(workspace) || !slackUserID.MatchString(external) || !strings.HasPrefix(text, "bind ") || !bindingToken.MatchString(strings.TrimPrefix(text, "bind ")) {
		return 0, ErrConflict
	}
	token := strings.TrimPrefix(text, "bind ")
	digest := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(digest[:])
	var actor int64
	if err := tx.QueryRowContext(ctx, `SELECT c.user_id FROM platform_bot_binding_challenges c JOIN platform_users u ON u.id=c.user_id AND u.disabled=0 WHERE c.integration_id=? AND c.integration_revision=? AND c.token_hash=? AND c.expires_at>?`, integrationID, revision, hash, at.Unix()).Scan(&actor); err != nil {
		return 0, err
	}
	// Never overwrite another external identity or platform account binding.
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_bot_bindings(integration_id,workspace_id,external_user_id,user_id,created_at) VALUES(?,?,?,?,?)`, integrationID, workspace, external, actor, now())
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM platform_bot_binding_challenges WHERE integration_id=? AND user_id=? AND token_hash=?`, integrationID, actor, hash); err != nil {
		return 0, err
	}
	return actor, tx.Commit()
}
