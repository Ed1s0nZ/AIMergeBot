package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

// The credentials remain internal. Every authorization observation uses the
// same read snapshot, and is repeated by the sender before and after POST.
func (s *Store) TicketSendContext(ctx context.Context, claim TicketClaim) (Snapshot, IntegrationCredentials, error) {
	failed := func(err error) (Snapshot, IntegrationCredentials, error) {
		return Snapshot{}, IntegrationCredentials{}, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return failed(err)
	}
	defer tx.Rollback()
	snap, err := requireDispositionFinding(ctx, tx, claim.RunID, claim.Actor, claim.FindingID, "operator")
	if err != nil {
		return failed(err)
	}
	if claim.HeadSHA != snap.HeadSHA || !commitID.MatchString(snap.HeadSHA) {
		return failed(ErrConflict)
	}
	if err := requireTicketProjectsEnabled(ctx, tx, snap); err != nil {
		return failed(err)
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_ticket_links WHERE id=? AND run_id=? AND finding_id=? AND integration_id=? AND integration_revision=? AND provider=? AND actor=? AND head_sha=? AND idempotency_key=? AND state='sending' AND lease=? AND lease!='' AND julianday(lease_until)>julianday(?))`, claim.ID, claim.RunID, claim.FindingID, claim.IntegrationID, claim.IntegrationRevision, claim.Provider, claim.Actor, claim.HeadSHA, claim.IdempotencyKey, claim.Lease, now()).Scan(&valid); err != nil {
		return failed(err)
	}
	if !valid {
		return failed(ErrConflict)
	}
	var revision int64
	var provider, scopeJSON, credentialsJSON string
	var enabled bool
	if err := tx.QueryRowContext(ctx, `SELECT revision,kind,enabled,project_ids,CASE WHEN length(CAST(credentials AS BLOB))<=65536 THEN credentials ELSE '' END FROM platform_integrations WHERE id=?`, claim.IntegrationID).Scan(&revision, &provider, &enabled, &scopeJSON, &credentialsJSON); err != nil {
		return failed(err)
	}
	if !enabled || revision != claim.IntegrationRevision || provider != claim.Provider || (provider != "linear" && provider != "jira") {
		return failed(ErrConflict)
	}
	var scope []int
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return failed(err)
	}
	inScope := false
	for _, project := range scope {
		if project == snap.ProjectID {
			inScope = true
		}
	}
	if !inScope {
		return failed(ErrProjectPermission)
	}
	var credentials IntegrationCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return failed(err)
	}
	if err := validateIntegrationCredentials(provider, credentials, true); err != nil {
		return failed(err)
	}
	if claim.Provider == "jira" && (claim.EndpointOrigin == "" || credentials.Endpoint != claim.EndpointOrigin) {
		return failed(ErrConflict)
	}
	if err := tx.Commit(); err != nil {
		return failed(err)
	}
	return snap, credentials, nil
}

func requireTicketProjectsEnabled(ctx context.Context, tx *sql.Tx, snap Snapshot) error {
	projects := []int{snap.ProjectID, snap.SourceProjectID}
	for _, item := range contextPolicyItems(snap) {
		projects = append(projects, item.ProjectID)
	}
	for _, project := range projects {
		var enabled bool
		if err := tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, project).Scan(&enabled); err != nil {
			return err
		}
		if !enabled {
			return ErrProjectPermission
		}
	}
	return nil
}
