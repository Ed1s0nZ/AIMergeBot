package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Keys bind one server-generated operation's inputs. A replay returns metadata,
// never a fresh permission to perform another physical read or DP computation.
type repositoryBudgetReservationKey struct {
	id, nonce, budget, kind string
	epoch, requested        int64
}
type repositoryBudgetReceipt struct {
	id, budget, owner, kind, state string
	epoch, reserved                int64
	replayed                       bool
}

func (repositoryBudgetReservationKey) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository budget reservation key]")
}
func (repositoryBudgetReceipt) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository budget receipt]")
}

func (k repositoryBudgetReservationKey) digest() string {
	return repositoryBudgetDigest([]byte(fmt.Sprintf("v1:%s:%d:%s:%d:%s", k.budget, k.epoch, k.kind, k.requested, k.nonce)))
}

func newRepositoryBudgetReservationKey(h repositoryBudgetAdmission, kind string, requested int64) (repositoryBudgetReservationKey, error) {
	k := repositoryBudgetReservationKey{budget: h.id, epoch: h.epoch, kind: kind, requested: requested}
	if err := k.validate(h); err != nil {
		return k, err
	}
	var err error
	k.nonce, err = newWorkerOwner()
	if err != nil {
		return repositoryBudgetReservationKey{}, errRepositoryBudgetAccounting
	}
	k.id = k.digest()
	return k, nil
}

func (k repositoryBudgetReservationKey) validate(h repositoryBudgetAdmission) error {
	if k.budget != h.id || k.epoch != h.epoch || k.requested <= 0 {
		return ErrRepositoryBudget
	}
	switch k.kind {
	case "request":
		if k.requested != 1 {
			return ErrRepositoryBudget
		}
	case "api_read":
		if k.requested > h.limits.PerAPIResponseBytes+1 {
			return ErrRepositoryBudget
		}
	case "git_read":
		if h.limits.PerGitResponseBytes == 0 {
			return errRepositoryBudgetExhausted
		}
		if k.requested > h.limits.PerGitResponseBytes+1 {
			return ErrRepositoryBudget
		}
	case "upload":
		if h.limits.UploadBytes == 0 {
			return errRepositoryBudgetExhausted
		}
		if k.requested > 1<<20 {
			return ErrRepositoryBudget
		}
	case "text_match":
		if h.limits.PerTextMatchCells == 0 {
			return errRepositoryBudgetExhausted
		}
		if k.requested > h.limits.PerTextMatchCells {
			return ErrRepositoryBudget
		}
	default:
		return ErrRepositoryBudget
	}
	return nil
}

func budgetAffectedOne(ctx context.Context, result sql.Result, err error) error {
	if err != nil {
		return budgetAccountingError(ctx)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return budgetAccountingError(ctx)
	}
	if n != 1 {
		return errRepositoryBudgetLease
	}
	return nil
}

func loadBudgetReceipt(ctx context.Context, tx *sql.Tx, id string) (repositoryBudgetReceipt, sql.NullInt64, error) {
	r := repositoryBudgetReceipt{id: id}
	var used, run sql.NullInt64
	var ownerKind string
	var settled sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT budget_id,owner_kind,owner,lease_epoch,run_id,kind,reserved,state,settled_used,settled_at FROM platform_repository_budget_reservations WHERE id=?`, id).Scan(&r.budget, &ownerKind, &r.owner, &r.epoch, &run, &r.kind, &r.reserved, &r.state, &used, &settled)
	if err != nil {
		return r, used, err
	}
	if ownerKind != "admission" || run.Valid || r.reserved <= 0 || (r.state != "open" && r.state != "settled" && r.state != "charged_unknown") || (r.state == "open" && (used.Valid || settled.Valid)) || (r.state == "settled" && (!used.Valid || !settled.Valid || used.Int64 < 0 || used.Int64 > r.reserved)) || (r.state == "charged_unknown" && (used.Valid || !settled.Valid)) {
		return r, used, errRepositoryBudgetAccounting
	}
	return r, used, nil
}

func receiptMatchesAdmission(r repositoryBudgetReceipt, h repositoryBudgetAdmission) bool {
	return r.budget == h.id && r.owner == h.owner && r.epoch == h.epoch
}

// requested is a per-response remaining bound for reads, exact bytes for
// upload, or exact cells. The ledger computes a full canonical read grant.
func (s *Store) reserveRepositoryBudget(ctx context.Context, h repositoryBudgetAdmission, k repositoryBudgetReservationKey) (repositoryBudgetReceipt, error) {
	var zero repositoryBudgetReceipt
	if err := k.validate(h); err != nil {
		return zero, err
	}
	if len(k.nonce) != 64 || k.id != k.digest() {
		return zero, ErrRepositoryBudget
	}
	operation, err := h.operationContext(ctx)
	if err != nil {
		return zero, err
	}
	ctx = operation
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return zero, budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	row, err := s.loadBudgetAdmissionTx(ctx, tx, h, true)
	if err != nil {
		return zero, err
	}
	existing, _, err := loadBudgetReceipt(ctx, tx, k.id)
	if err == nil {
		if !receiptMatchesAdmission(existing, h) || existing.kind != k.kind || existing.state == "charged_unknown" {
			return zero, errRepositoryBudgetLease
		}
		if k.kind == "api_read" || k.kind == "git_read" {
			if existing.reserved > min(h.limits.GrantBytes, k.requested) {
				return zero, ErrRepositoryBudget
			}
		} else if existing.reserved != k.requested {
			return zero, ErrRepositoryBudget
		}
		if err = tx.Commit(); err != nil {
			return zero, budgetAccountingError(ctx)
		}
		if err = h.checkOperation(ctx); err != nil {
			return zero, err
		}
		existing.replayed = true
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return zero, budgetAccountingError(ctx)
	}
	var count int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_repository_budget_reservations WHERE budget_id=?`, h.id).Scan(&count); err != nil {
		return zero, budgetAccountingError(ctx)
	}
	if count >= h.limits.MaxReservations {
		return zero, errRepositoryBudgetExhausted
	}
	amount := k.requested
	var sets, condition string
	var args []any
	switch k.kind {
	case "request":
		if row.charges.requests >= h.limits.Requests {
			return zero, errRepositoryBudgetExhausted
		}
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_repository_budget_reservations WHERE budget_id=? AND owner_kind='admission' AND owner=? AND lease_epoch=? AND kind='request' AND state='open'`, h.id, h.owner, h.epoch).Scan(&count); err != nil {
			return zero, budgetAccountingError(ctx)
		}
		if count >= h.limits.ParallelRequests {
			return zero, errRepositoryBudgetBusy
		}
		sets, condition = "requests_charged=requests_charged+?", "requests_charged<=?"
		args = []any{amount, h.limits.Requests - amount}
	case "api_read", "git_read":
		column, available := "api_charged", h.limits.APIResponseBytes-row.charges.api
		if k.kind == "git_read" {
			column, available = "git_charged", h.limits.GitResponseBytes-row.charges.git
		}
		amount = min(h.limits.GrantBytes, k.requested, available, h.limits.DownloadBytes-row.charges.download)
		if amount <= 0 {
			return zero, errRepositoryBudgetExhausted
		}
		sets = column + "=" + column + "+?,download_charged=download_charged+?"
		condition = column + "<=? AND download_charged<=?"
		limit := h.limits.APIResponseBytes
		if k.kind == "git_read" {
			limit = h.limits.GitResponseBytes
		}
		args = []any{amount, amount, limit - amount, h.limits.DownloadBytes - amount}
	case "upload", "text_match":
		column, available, limit := "upload_charged", h.limits.UploadBytes-row.charges.upload, h.limits.UploadBytes
		if k.kind == "text_match" {
			column, available, limit = "match_cells_charged", h.limits.TextMatchCells-row.charges.cells, h.limits.TextMatchCells
		}
		if amount > available {
			return zero, errRepositoryBudgetExhausted
		}
		sets, condition = column+"="+column+"+?", column+"<=?"
		args = []any{amount, limit - amount}
	}
	// Separate SET and predicate arguments while keeping every bound parameterized.
	setCount := 1
	if k.kind == "api_read" || k.kind == "git_read" {
		setCount = 2
	}
	params := append([]any{}, args[:setCount]...)
	params = append(params, now(), h.id, h.owner, h.epoch)
	params = append(params, args[setCount:]...)
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET `+sets+`,updated_at=? WHERE id=? AND admission_owner=? AND session_epoch=? AND state='admitting' AND julianday(admission_lease_until)>julianday('now') AND julianday(admission_deadline)>julianday('now') AND `+condition, params...)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return zero, err
	}
	r := repositoryBudgetReceipt{id: k.id, budget: h.id, owner: h.owner, epoch: h.epoch, kind: k.kind, reserved: amount, state: "open"}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_repository_budget_reservations(id,budget_id,owner_kind,owner,lease_epoch,kind,reserved,state,created_at) VALUES(?,?,'admission',?,?,?,?,'open',?)`, r.id, r.budget, r.owner, r.epoch, r.kind, r.reserved, now()); err != nil {
		return zero, budgetAccountingError(ctx)
	}
	if err = tx.Commit(); err != nil {
		return zero, budgetAccountingError(ctx)
	}
	if err = h.checkOperation(ctx); err != nil {
		return zero, err
	}
	return r, nil
}

func (s *Store) settleRepositoryBudget(ctx context.Context, h repositoryBudgetAdmission, r repositoryBudgetReceipt, used int64) error {
	if !receiptMatchesAdmission(r, h) || used < 0 || used > r.reserved || ((r.kind == "request" || r.kind == "text_match") && used != r.reserved) {
		return ErrRepositoryBudget
	}
	operation, err := h.operationContext(ctx)
	if err != nil {
		return err
	}
	ctx = operation
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	if _, err = s.loadBudgetAdmissionTx(ctx, tx, h, true); err != nil {
		return err
	}
	stored, previous, err := loadBudgetReceipt(ctx, tx, r.id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRepositoryBudget
		}
		return budgetAccountingError(ctx)
	}
	if !receiptMatchesAdmission(stored, h) || stored.kind != r.kind || stored.reserved != r.reserved {
		return ErrRepositoryBudget
	}
	if stored.state == "charged_unknown" {
		return errRepositoryBudgetLease
	}
	if stored.state == "settled" {
		if previous.Int64 != used {
			return ErrRepositoryBudget
		}
		return nil
	}
	refund := r.reserved - used
	if refund > 0 {
		var sets, condition string
		var args []any
		switch r.kind {
		case "api_read", "git_read":
			column := "api_charged"
			if r.kind == "git_read" {
				column = "git_charged"
			}
			sets = column + "=" + column + "-?,download_charged=download_charged-?"
			condition = column + ">=? AND download_charged>=?"
			args = []any{refund, refund, now(), h.id, h.owner, h.epoch, refund, refund}
		case "upload":
			sets = "upload_charged=upload_charged-?"
			condition = "upload_charged>=?"
			args = []any{refund, now(), h.id, h.owner, h.epoch, refund}
		default:
			return ErrRepositoryBudget
		}
		result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET `+sets+`,updated_at=? WHERE id=? AND admission_owner=? AND session_epoch=? AND state='admitting' AND julianday(admission_lease_until)>julianday('now') AND julianday(admission_deadline)>julianday('now') AND `+condition, args...)
		if err = budgetAffectedOne(ctx, result, err); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budget_reservations SET state='settled',settled_used=?,settled_at=? WHERE id=? AND budget_id=? AND owner_kind='admission' AND owner=? AND lease_epoch=? AND state='open' AND EXISTS(SELECT 1 FROM platform_repository_budgets b WHERE b.id=budget_id AND b.state='admitting' AND b.admission_owner=? AND b.session_epoch=? AND julianday(b.admission_lease_until)>julianday('now') AND julianday(b.admission_deadline)>julianday('now'))`, used, now(), r.id, h.id, h.owner, h.epoch, h.owner, h.epoch)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return budgetAccountingError(ctx)
	}
	return h.checkOperation(ctx)
}
