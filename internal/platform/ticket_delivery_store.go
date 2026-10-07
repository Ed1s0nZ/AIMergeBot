package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
)

type TicketClaim struct {
	TicketLink
	Lease string
}

func (s *Store) ClaimFindingTicket(ctx context.Context) (TicketClaim, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TicketClaim{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE platform_ticket_links SET state='unknown',error_code='creation_unacknowledged',lease='',lease_until='',updated_at=? WHERE state='sending' AND (julianday(lease_until) IS NULL OR julianday(lease_until)<=julianday(?))`, now(), now()); err != nil {
		return TicketClaim{}, err
	}
	link, err := scanTicketLink(tx.QueryRowContext(ctx, `SELECT `+ticketLinkColumns+` FROM platform_ticket_links WHERE state='pending' ORDER BY id LIMIT 1`))
	if err == sql.ErrNoRows {
		if commitErr := tx.Commit(); commitErr != nil {
			return TicketClaim{}, commitErr
		}
		return TicketClaim{}, err
	}
	if err != nil {
		return TicketClaim{}, err
	}
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		return TicketClaim{}, err
	}
	lease := hex.EncodeToString(seed)
	if _, err := tx.ExecContext(ctx, `UPDATE platform_ticket_links SET state='sending',lease=?,lease_until=?,updated_at=? WHERE id=? AND state='pending'`, lease, time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano), now(), link.ID); err != nil {
		return TicketClaim{}, err
	}
	link.State = "sending"
	return TicketClaim{TicketLink: link, Lease: lease}, tx.Commit()
}

func (s *Store) FinishFindingTicket(ctx context.Context, claim TicketClaim, receipt TicketReceipt) error {
	if receipt.State != "created" && receipt.State != "unknown" && receipt.State != "failed" {
		return ErrConflict
	}
	if receipt.RemoteID != "" || receipt.URL != "" {
		if receipt.State == "failed" || claim.Provider != "linear" || !validLinearTicketIdentity(receipt.RemoteID, receipt.URL) {
			return ErrConflict
		}
	}
	if receipt.State == "created" && (claim.Provider != "linear" || !validLinearTicketIdentity(receipt.RemoteID, receipt.URL)) {
		return ErrConflict
	}
	switch receipt.Code {
	case "", "invalid_configuration", "permission_changed", "creation_unacknowledged", "unsupported_provider":
	default:
		return ErrConflict
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE platform_ticket_links SET state=?,remote_id=?,url=?,error_code=?,lease='',lease_until='',updated_at=? WHERE id=? AND run_id=? AND finding_id=? AND integration_id=? AND integration_revision=? AND provider=? AND actor=? AND head_sha=? AND idempotency_key=? AND state='sending' AND lease=? AND lease!='' AND julianday(lease_until)>julianday(?)`, receipt.State, receipt.RemoteID, receipt.URL, receipt.Code, now(), claim.ID, claim.RunID, claim.FindingID, claim.IntegrationID, claim.IntegrationRevision, claim.Provider, claim.Actor, claim.HeadSHA, claim.IdempotencyKey, claim.Lease, now())
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
	return nil
}
