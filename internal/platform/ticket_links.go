package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
)

type TicketLink struct {
	ID                  int64  `json:"id"`
	RunID               int64  `json:"run_id"`
	FindingID           string `json:"finding_id"`
	IntegrationID       int64  `json:"integration_id"`
	IntegrationRevision int64  `json:"integration_revision"`
	Provider            string `json:"provider"`
	HeadSHA             string `json:"head_sha"`
	Actor               int64  `json:"actor"`
	State               string `json:"state"`
	RemoteID            string `json:"remote_id,omitempty"`
	URL                 string `json:"url,omitempty"`
	IdempotencyKey      string `json:"-"`
}

func migrateTicketLinks(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_ticket_links(id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,integration_id INTEGER NOT NULL REFERENCES platform_integrations(id),integration_revision INTEGER NOT NULL,provider TEXT NOT NULL,head_sha TEXT NOT NULL,actor INTEGER NOT NULL REFERENCES platform_users(id),state TEXT NOT NULL DEFAULT 'pending',remote_id TEXT NOT NULL DEFAULT '',url TEXT NOT NULL DEFAULT '',idempotency_key TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(run_id,finding_id,integration_id))`)
	return err
}

const ticketLinkColumns = `id,run_id,finding_id,integration_id,integration_revision,provider,head_sha,actor,state,remote_id,url,idempotency_key`

func scanTicketLink(row interface{ Scan(...any) error }) (TicketLink, error) {
	var v TicketLink
	err := row.Scan(&v.ID, &v.RunID, &v.FindingID, &v.IntegrationID, &v.IntegrationRevision, &v.Provider, &v.HeadSHA, &v.Actor, &v.State, &v.RemoteID, &v.URL, &v.IdempotencyKey)
	return v, err
}

func (s *Store) ReserveFindingTicket(ctx context.Context, runID, actor, integrationID, expectedRevision int64, findingID, headSHA string) (TicketLink, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TicketLink{}, false, err
	}
	defer tx.Rollback()
	snap, err := requireDispositionFinding(ctx, tx, runID, actor, findingID, "operator")
	if err != nil {
		return TicketLink{}, false, err
	}
	if headSHA != snap.HeadSHA || !commitID.MatchString(headSHA) {
		return TicketLink{}, false, ErrConflict
	}
	var provider, projects string
	var revision int64
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT kind,project_ids,revision,enabled FROM platform_integrations WHERE id=?`, integrationID).Scan(&provider, &projects, &revision, &enabled); err != nil {
		return TicketLink{}, false, err
	}
	if !enabled || revision != expectedRevision || (provider != "jira" && provider != "linear") {
		return TicketLink{}, false, ErrConflict
	}
	var scope []int
	if err := json.Unmarshal([]byte(projects), &scope); err != nil {
		return TicketLink{}, false, err
	}
	inScope := false
	for _, project := range scope {
		if project == snap.ProjectID {
			inScope = true
		}
	}
	if !inScope {
		return TicketLink{}, false, ErrProjectPermission
	}
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		return TicketLink{}, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_ticket_links(run_id,finding_id,integration_id,integration_revision,provider,head_sha,actor,idempotency_key,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, runID, findingID, integrationID, revision, provider, headSHA, actor, hex.EncodeToString(seed), now(), now())
	if err != nil {
		return TicketLink{}, false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return TicketLink{}, false, err
	}
	link, err := scanTicketLink(tx.QueryRowContext(ctx, `SELECT `+ticketLinkColumns+` FROM platform_ticket_links WHERE run_id=? AND finding_id=? AND integration_id=?`, runID, findingID, integrationID))
	if err != nil {
		return TicketLink{}, false, err
	}
	return link, inserted == 1, tx.Commit()
}

func (s *Store) FindingTicket(ctx context.Context, runID, actor, integrationID int64, findingID string) (TicketLink, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TicketLink{}, err
	}
	defer tx.Rollback()
	if _, err := requireDispositionFinding(ctx, tx, runID, actor, findingID, "viewer"); err != nil {
		return TicketLink{}, err
	}
	v, err := scanTicketLink(tx.QueryRowContext(ctx, `SELECT `+ticketLinkColumns+` FROM platform_ticket_links WHERE run_id=? AND finding_id=? AND integration_id=?`, runID, findingID, integrationID))
	if err != nil {
		return TicketLink{}, err
	}
	return v, tx.Commit()
}
