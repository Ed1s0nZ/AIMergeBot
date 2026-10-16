package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const repositoryPolicyVersion = "eino-audit-contract-v51"

type repositoryBudgetWorkerSession struct {
	store                       *Store
	origin                      context.Context
	budget, owner, policyDigest string
	run, epoch                  int64
}

func (repositoryBudgetWorkerSession) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository budget worker session]")
}

// Internal producer entry only. Public legacy enqueue remains fail closed;
// Factory metadata observation and all new ingress are still not mounted.
func (s *Store) enqueueRepositoryBudgetRun(ctx context.Context, h repositoryBudgetAdmission, snap Snapshot, force bool) (int64, bool, error) {
	ctx, err := h.operationContext(ctx)
	if err != nil {
		return 0, false, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	if _, err = s.loadBudgetAdmissionTx(ctx, tx, h, true); err != nil {
		return 0, false, err
	}
	if h.purpose != "audit" || snap.ProjectID != h.project || snap.AuditPolicy == nil || snap.AuditPolicy.RepositoryBudget == nil || *snap.AuditPolicy.RepositoryBudget != h.limits || snap.AuditPolicy.Git != h.policy.Git || snap.AuditPolicy.TimeoutSeconds != h.policy.TimeoutSeconds {
		return 0, false, ErrRepositoryBudget
	}
	if err = s.authorizeRepositoryBudgetSnapshot(ctx, tx, snap, h.actor); err != nil {
		return 0, false, err
	}
	if snap.AuditPolicy.FollowupOf != 0 {
		return 0, false, ErrFollowupScope
	} // Dedicated follow-up producer is not yet integrated.
	raw, err := json.Marshal(snap.AuditPolicy)
	if err != nil {
		return 0, false, ErrRepositoryBudget
	}
	query := `SELECT id FROM platform_runs WHERE project_id=? AND mr_iid=? AND base_sha=? AND head_sha=? AND policy_version=? AND policy_digest=?`
	if force {
		query += ` AND status IN ('pending','running')`
	} else {
		query += ` AND status IN ('pending','running','succeeded','skipped')`
	}
	query += ` ORDER BY id DESC LIMIT 1`
	var id int64
	err = tx.QueryRowContext(ctx, query, snap.ProjectID, snap.MRIID, snap.BaseSHA, snap.HeadSHA, repositoryPolicyVersion, policyDigest(snap.AuditPolicy)).Scan(&id)
	if err == nil {
		if _, _, _, err = s.boundRepositoryBudgetForReuseTx(ctx, tx, id, true); err != nil {
			return 0, false, err
		}
		if err = s.endRepositoryBudgetAdmissionTx(ctx, tx, h); err != nil {
			return 0, false, err
		}
		if err = tx.Commit(); err != nil {
			return 0, false, budgetAccountingError(ctx)
		}
		if err = h.checkOperation(ctx); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, budgetAccountingError(ctx)
	}
	if err = checkOutstanding(ctx, tx, snap.ProjectID, h.actor, s.auditQuotas()); err != nil {
		return 0, false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version,policy_digest,audit_policy_json) VALUES(?,?,?,?,?,?,?,?,'pending',?,?,?,?,?)`, snap.ProjectID, snap.MRIID, snap.SourceProjectID, snap.DiffVersionID, snap.BaseSHA, snap.HeadSHA, snap.Title, snap.URL, now(), h.actor, repositoryPolicyVersion, policyDigest(snap.AuditPolicy), string(raw))
	if err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	id, err = result.LastInsertId()
	if err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	if err = insertRunContextRepositories(ctx, tx, id, snap.AuditPolicy); err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_repository_budget_reservations SET state='charged_unknown',settled_at=? WHERE budget_id=? AND state='open'`, now(), h.id); err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	result, err = tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET state='active',root_run_id=?,admission_owner='',admission_lease_until='',updated_at=? WHERE id=? AND state='admitting' AND admission_owner=? AND session_epoch=? AND julianday(admission_lease_until)>julianday('now') AND julianday(admission_deadline)>julianday('now')`, id, now(), h.id, h.owner, h.epoch)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_run_repository_budgets(run_id,budget_id) VALUES(?,?)`, id, h.id); err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'run.created',?,?)`, h.actor, fmt.Sprint(id), now()); err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	if err = tx.Commit(); err != nil {
		return 0, false, budgetAccountingError(ctx)
	}
	if err = h.checkOperation(ctx); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Store) repositoryBudgetRunSnapshotTx(ctx context.Context, tx *sql.Tx, run int64) (Snapshot, int64, error) {
	var version, digest string
	var actor int64
	if err := tx.QueryRowContext(ctx, `SELECT policy_version,requested_by,policy_digest FROM platform_runs WHERE id=?`, run).Scan(&version, &actor, &digest); err != nil {
		return Snapshot{}, 0, errRepositoryBudgetAccounting
	}
	snap, err := snapshotForRun(ctx, tx, run)
	if err != nil || version != repositoryPolicyVersion || digest != policyDigest(snap.AuditPolicy) {
		return Snapshot{}, 0, ErrRepositoryUnavailable
	}
	if err = s.authorizeRepositoryBudgetSnapshot(ctx, tx, snap, actor); err != nil {
		return Snapshot{}, 0, err
	}
	if validateRepositoryScopeContextIndex(ctx, tx, run, snap) != nil {
		return Snapshot{}, 0, ErrRepositoryUnavailable
	}
	return snap, actor, nil
}

// Validate the full existing retry chain, not just the newest row's budget ID.
func (s *Store) boundRepositoryBudgetTx(ctx context.Context, tx *sql.Tx, run int64) (string, int64, Snapshot, error) {
	return s.boundRepositoryBudgetForReuseTx(ctx, tx, run, false)
}

// A closed ledger may identify a completed duplicate, but never grants worker
// authority. Both paths validate the original root and every retry mapping.
func (s *Store) boundRepositoryBudgetForReuseTx(ctx context.Context, tx *sql.Tx, run int64, reuse bool) (string, int64, Snapshot, error) {
	snap, actor, err := s.repositoryBudgetRunSnapshotTx(ctx, tx, run)
	if err != nil {
		return "", 0, snap, err
	}
	var id, state, purpose, principal, pd, limits, ld, status string
	var project int
	var user, root, related sql.NullInt64
	var epoch int64
	err = tx.QueryRowContext(ctx, `SELECT b.id,b.state,b.purpose,b.project_id,b.actor_id,b.root_run_id,b.related_run_id,b.principal_json,b.principal_digest,b.limits_json,b.limits_digest,b.session_epoch,r.status FROM platform_repository_budgets b JOIN platform_run_repository_budgets m ON m.budget_id=b.id JOIN platform_runs r ON r.id=m.run_id WHERE m.run_id=?`, run).Scan(&id, &state, &purpose, &project, &user, &root, &related, &principal, &pd, &limits, &ld, &epoch, &status)
	if err != nil {
		return "", 0, snap, errRepositoryBudgetAccounting
	}
	var decoded RepositoryBudgetLimits
	validState := state == "active"
	if reuse && (status == "succeeded" || status == "skipped") {
		validState = state == "closed"
	}
	if !validState || purpose != "audit" || project != snap.ProjectID || !user.Valid || user.Int64 != actor || !root.Valid || root.Int64 <= 0 || related.Valid || epoch < 0 || principal != repositoryBudgetUserPrincipal(actor) || pd != repositoryBudgetDigest([]byte(principal)) || json.Unmarshal([]byte(limits), &decoded) != nil || decoded != *snap.AuditPolicy.RepositoryBudget || ld != repositoryBudgetDigest([]byte(limits)) {
		return "", 0, snap, errRepositoryBudgetAccounting
	}
	ancestors, err := retryBudgetAncestors(ctx, tx, run)
	if err != nil || ancestors[len(ancestors)-1].id != root.Int64 {
		return "", 0, snap, errRepositoryBudgetAccounting
	}
	for _, a := range ancestors {
		var mapped string
		if tx.QueryRowContext(ctx, `SELECT budget_id FROM platform_run_repository_budgets WHERE run_id=?`, a.id).Scan(&mapped) != nil || mapped != id {
			return "", 0, snap, errRepositoryBudgetAccounting
		}
	}
	return id, epoch, snap, nil
}

func (s *Store) openRepositoryBudgetWorker(ctx context.Context, run int64, owner string) (repositoryBudgetWorkerSession, error) {
	var w repositoryBudgetWorkerSession
	if owner == "" {
		return w, ErrWorkerLeaseLost
	}
	if err := checkRepositoryFence(ctx); err != nil {
		return w, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return w, budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	if err = repositoryBudgetWorkerLeaseTx(ctx, tx, run, owner); err != nil {
		return w, err
	}
	id, epoch, snap, err := s.boundRepositoryBudgetTx(ctx, tx, run)
	if err != nil {
		return w, err
	}
	if epoch == math.MaxInt64 {
		return w, errRepositoryBudgetLease
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_repository_budget_reservations SET state='charged_unknown',settled_at=? WHERE budget_id=? AND state='open'`, now(), id); err != nil {
		return w, budgetAccountingError(ctx)
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET session_epoch=session_epoch+1,updated_at=? WHERE id=? AND state='active' AND session_epoch=?`, now(), id, epoch)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return w, err
	}
	if err = tx.Commit(); err != nil {
		return w, budgetAccountingError(ctx)
	}
	if err = checkRepositoryFence(ctx); err != nil {
		return w, err
	}
	return repositoryBudgetWorkerSession{store: s, origin: ctx, budget: id, owner: owner, run: run, epoch: epoch + 1, policyDigest: policyDigest(snap.AuditPolicy)}, nil
}

func repositoryBudgetWorkerLeaseTx(ctx context.Context, tx *sql.Tx, run int64, owner string) error {
	if owner == "" {
		return ErrWorkerLeaseLost
	}
	if _, err := currentWorkerLease(ctx, tx, owner); err != nil {
		return ErrWorkerLeaseLost
	}
	var valid bool
	if tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_runs r WHERE r.id=? AND r.status='running' AND r.worker_owner=? AND julianday(r.worker_lease_until)>julianday('now') AND NOT EXISTS(SELECT 1 FROM platform_runs child WHERE child.retry_parent_id=r.id))`, run, owner).Scan(&valid) != nil || !valid {
		return ErrWorkerLeaseLost
	}
	return nil
}

func (s *Store) validateRepositoryBudgetWorker(ctx context.Context, w repositoryBudgetWorkerSession) error {
	if w.store != s || w.origin == nil {
		return errRepositoryBudgetLease
	}
	if err := checkRepositoryFence(ctx); err != nil {
		return err
	}
	if err := checkRepositoryFence(w.origin); err != nil {
		return err
	}
	ctx = copyRepositoryCredentials(ctx, w.origin)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	if err = repositoryBudgetWorkerLeaseTx(ctx, tx, w.run, w.owner); err != nil {
		return err
	}
	id, epoch, snap, err := s.boundRepositoryBudgetTx(ctx, tx, w.run)
	if err != nil {
		return err
	}
	if id != w.budget || epoch != w.epoch || policyDigest(snap.AuditPolicy) != w.policyDigest {
		return errRepositoryBudgetLease
	}
	if err = tx.Commit(); err != nil {
		return budgetAccountingError(ctx)
	}
	if err = checkRepositoryFence(w.origin); err != nil {
		return err
	}
	return checkRepositoryFence(ctx)
}

// Caller inserts and transitions the child in the same transaction. This is
// deliberately not a public "recover by budget ID" or a retry scheduler.
func (s *Store) inheritRepositoryRetryBudgetTx(ctx context.Context, tx *sql.Tx, parent, child int64) error {
	id, epoch, _, err := s.boundRepositoryBudgetTx(ctx, tx, parent)
	if err != nil {
		return err
	}
	_, _, err = s.repositoryBudgetRunSnapshotTx(ctx, tx, child)
	if err != nil {
		return err
	}
	ancestors, err := retryBudgetAncestors(ctx, tx, child)
	if err != nil || len(ancestors) < 2 || ancestors[1].id != parent || ancestors[0].status != "pending" || ancestors[1].status != "failed" {
		return ErrConflict
	}
	if epoch == math.MaxInt64 {
		return errRepositoryBudgetLease
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_run_repository_budgets(run_id,budget_id) VALUES(?,?)`, child, id); err != nil {
		return budgetAccountingError(ctx)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_repository_budget_reservations SET state='charged_unknown',settled_at=? WHERE budget_id=? AND state='open'`, now(), id); err != nil {
		return budgetAccountingError(ctx)
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET session_epoch=session_epoch+1,updated_at=? WHERE id=? AND session_epoch=? AND state='active'`, now(), id, epoch)
	return budgetAffectedOne(ctx, result, err)
}
