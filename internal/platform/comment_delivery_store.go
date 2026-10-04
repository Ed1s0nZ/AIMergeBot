package platform

import (
	"context"
	"fmt"
	"time"
)

type CommentDelivery struct {
	RunID             int64  `json:"run_id"`
	DesiredGeneration int    `json:"desired_generation"`
	SentGeneration    int    `json:"sent_generation"`
	State             string `json:"state"`
	DiscussionID      string `json:"discussion_id,omitempty"`
	NoteID            int    `json:"note_id,omitempty"`
	AuthorID          int    `json:"-"`
	BodyHash          string `json:"-"`
	AttemptedHash     string `json:"-"`
	ClaimedGeneration int    `json:"-"`
	LastError         string `json:"reason,omitempty"`
	ClaimOwner        string `json:"-"`
	ClaimUntil        string `json:"-"`
	Attempts          int    `json:"-"`
	ReviewActor       int64  `json:"-"`
	UpdatedAt         string `json:"updated_at"`
}

const deliveryColumns = `run_id,desired_generation,sent_generation,state,discussion_id,note_id,author_id,body_hash,attempted_hash,claimed_generation,last_error,claim_owner,claim_until,attempts,review_actor,updated_at`

func scanDelivery(row interface{ Scan(...any) error }) (CommentDelivery, error) {
	var d CommentDelivery
	err := row.Scan(&d.RunID, &d.DesiredGeneration, &d.SentGeneration, &d.State, &d.DiscussionID, &d.NoteID, &d.AuthorID, &d.BodyHash, &d.AttemptedHash, &d.ClaimedGeneration, &d.LastError, &d.ClaimOwner, &d.ClaimUntil, &d.Attempts, &d.ReviewActor, &d.UpdatedAt)
	return d, err
}
func (s *Store) CommentDelivery(ctx context.Context, id int64) (CommentDelivery, error) {
	return scanDelivery(s.DB.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM platform_comment_delivery WHERE run_id=?`, id))
}

const deliveryFence = ` AND claim_owner=? AND julianday(claim_until)>julianday('now') AND EXISTS(SELECT 1 FROM platform_worker_instance i WHERE i.id=1 AND i.owner=platform_comment_delivery.claim_owner AND julianday(i.lease_until)>julianday('now'))`

func (s *Store) claimCommentDelivery(ctx context.Context, owner string) (CommentDelivery, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return CommentDelivery{}, err
	}
	defer tx.Rollback()
	if owner == "" {
		return CommentDelivery{}, ErrWorkerLeaseLost
	}
	if _, err = currentWorkerLease(ctx, tx, owner); err != nil {
		return CommentDelivery{}, err
	}
	// Interrupted sends are ambiguous even when their owner no longer exists.
	if _, err = tx.ExecContext(ctx, `UPDATE platform_comment_delivery SET state='unknown',claim_owner='',claim_until='',last_error='Delivery interrupted; reconciliation required' WHERE state='sending' AND (julianday(claim_until)<=julianday('now') OR NOT EXISTS(SELECT 1 FROM platform_worker_instance i WHERE i.id=1 AND i.owner=platform_comment_delivery.claim_owner AND julianday(i.lease_until)>julianday('now')))`); err != nil {
		return CommentDelivery{}, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT run_id FROM platform_comment_delivery WHERE state IN ('pending','unknown') AND attempts<5 AND (retry_at='' OR julianday(retry_at)<=julianday('now')) ORDER BY run_id LIMIT 1`).Scan(&id)
	if err != nil {
		return CommentDelivery{}, err
	}
	until := time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano)
	// Preserve unknown classification in the returned work item.
	d, err := scanDelivery(tx.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM platform_comment_delivery WHERE run_id=?`, id))
	if err != nil {
		return d, err
	}
	generation := d.DesiredGeneration
	if d.State == "unknown" && d.ClaimedGeneration > 0 {
		generation = d.ClaimedGeneration
	}
	_, err = tx.ExecContext(ctx, `UPDATE platform_comment_delivery SET attempted_hash=CASE WHEN state='pending' THEN '' ELSE attempted_hash END,state='sending',claim_owner=?,claim_until=?,claimed_generation=?,attempts=attempts+1,updated_at=? WHERE run_id=?`, owner, until, generation, now(), id)
	if err != nil {
		return d, err
	}
	if d.State == "pending" {
		d.AttemptedHash = ""
	}
	d.ClaimOwner = owner
	d.ClaimUntil = until
	d.ClaimedGeneration = generation
	d.Attempts++
	return d, tx.Commit()
}
func (s *Store) prepareCommentBody(ctx context.Context, d CommentDelivery, hash string, authors ...int) error {
	author := 0
	if len(authors) > 0 {
		author = authors[0]
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_comment_delivery SET attempted_hash=?,author_id=CASE WHEN ? > 0 THEN ? ELSE author_id END WHERE run_id=? AND state='sending'`+deliveryFence, hash, author, author, d.RunID, d.ClaimOwner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) acknowledgeComment(ctx context.Context, d CommentDelivery, discussion string, note, author int, hash string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_comment_delivery SET discussion_id=?,note_id=?,author_id=?,body_hash=?,sent_generation=?,state=CASE WHEN desired_generation>? THEN 'pending' ELSE 'sent' END,attempts=0,last_error='',claim_owner='',claim_until='',retry_at='',updated_at=? WHERE run_id=? AND state='sending'`+deliveryFence, discussion, note, author, hash, d.ClaimedGeneration, d.ClaimedGeneration, now(), d.RunID, d.ClaimOwner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Store) deferComment(ctx context.Context, d CommentDelivery, state, reason string, causes ...error) error {
	switch state {
	case "pending", "unknown", "conflict", "stale", "blocked":
	default:
		return fmt.Errorf("invalid delivery state")
	}
	retry := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	stopRetries := false
	if len(causes) > 0 {
		if info, _ := retryFailure(causes[0]); info != nil {
			if info.HeaderState == "exceeds_limit" {
				stopRetries = true
			}
			if until, err := time.Parse(time.RFC3339Nano, info.RetryAfterUntil); err == nil && until.After(time.Now().UTC().Add(time.Minute)) {
				retry = until.Format(time.RFC3339Nano)
			}
		}
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_comment_delivery SET state=?,last_error=?,retry_at=?,attempts=CASE WHEN ? THEN 5 ELSE attempts END,claim_owner='',claim_until='',updated_at=? WHERE run_id=? AND state='sending'`+deliveryFence, state, reason, retry, stopRetries, now(), d.RunID, d.ClaimOwner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
