package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

type BotBindingChannel struct {
	ID       int64  `json:"id"`
	Revision int64  `json:"revision"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

const botChannelAccess = `EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(i.project_ids) THEN i.project_ids ELSE '[]' END) scope JOIN platform_projects p ON p.id=scope.value AND p.enabled=1 WHERE u.role='admin' OR EXISTS(SELECT 1 FROM platform_project_members m WHERE m.project_id=p.id AND m.user_id=u.id AND m.role IN ('viewer','reviewer','operator')))`

func slackBindingReady(c IntegrationCredentials) bool {
	return c.Secret != "" && len(c.Secret) <= 4096 && slackAppID.MatchString(c.SlackAppID) && slackWorkspaceID.MatchString(c.SlackWorkspaceID)
}

func (s *Store) BotBindingChannels(ctx context.Context, actor int64) ([]BotBindingChannel, error) {
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
	rows, err := tx.QueryContext(ctx, `SELECT i.id,i.revision,i.name,CASE WHEN length(CAST(i.credentials AS BLOB))<=65536 THEN i.credentials ELSE '' END FROM platform_integrations i JOIN platform_users u ON u.id=? AND u.disabled=0 WHERE i.kind='slack' AND i.enabled=1 AND `+botChannelAccess+` ORDER BY i.id LIMIT 501`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BotBindingChannel{}
	count := 0
	for rows.Next() {
		count++
		if count > 500 {
			return nil, ErrConflict
		}
		var v BotBindingChannel
		var raw string
		if err := rows.Scan(&v.ID, &v.Revision, &v.Name, &raw); err != nil {
			return nil, err
		}
		var c IntegrationCredentials
		if json.Unmarshal([]byte(raw), &c) != nil || !slackBindingReady(c) {
			continue
		}
		v.Provider = "slack"
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
