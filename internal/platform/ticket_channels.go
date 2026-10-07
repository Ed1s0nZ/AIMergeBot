package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

type TicketChannel struct {
	ID       int64  `json:"id"`
	Revision int64  `json:"revision"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// Return only usable mappings for this authorized finding. Credentials never
// leave this transaction; unsupported providers are not offered as choices.
func (s *Store) FindingTicketChannels(ctx context.Context, runID, actor int64, findingID string) ([]TicketChannel, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	snap, err := requireDispositionFinding(ctx, tx, runID, actor, findingID, "operator")
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,revision,name,kind,CASE WHEN length(CAST(credentials AS BLOB))<=65536 THEN credentials ELSE '' END FROM platform_integrations WHERE enabled=1 AND kind='linear' AND EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(project_ids) THEN project_ids ELSE '[]' END) WHERE value=?) ORDER BY id LIMIT 501`, snap.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketChannel{}
	count := 0
	for rows.Next() {
		count++
		if count > 500 {
			return nil, ErrConflict
		}
		var v TicketChannel
		var raw string
		if err := rows.Scan(&v.ID, &v.Revision, &v.Name, &v.Provider, &raw); err != nil {
			return nil, err
		}
		var credentials IntegrationCredentials
		if len(raw) > 65536 || json.Unmarshal([]byte(raw), &credentials) != nil {
			continue
		}
		if !linearTicketReady(credentials) {
			continue
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

func linearTicketReady(credentials IntegrationCredentials) bool {
	return credentials.Endpoint == "https://api.linear.app/graphql" && ticketUUID.MatchString(credentials.LinearTeamID) && credentials.Token != "" && credentials.Token == strings.TrimSpace(credentials.Token) && !strings.ContainsAny(credentials.Token, "\r\n") && validateIntegrationCredentials("linear", credentials, true) == nil
}
