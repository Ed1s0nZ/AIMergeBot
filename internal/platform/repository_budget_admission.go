package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	errRepositoryBudgetAccounting = errors.New("repository budget accounting unavailable")
	errRepositoryBudgetLease      = errors.New("repository budget lease lost")
	errRepositoryBudgetExhausted  = errors.New("repository budget exhausted")
	errRepositoryBudgetBusy       = errors.New("repository budget request slots busy")
)

type repositoryBudgetAdmission struct {
	store                                                                    *Store
	origin                                                                   context.Context
	id, owner, purpose, principal, principalDigest, limitsJSON, limitsDigest string
	actor                                                                    int64
	project                                                                  int
	epoch                                                                    int64
	deadline                                                                 time.Time
	policy                                                                   AuditPolicy
	limits                                                                   RepositoryBudgetLimits
}

func (repositoryBudgetAdmission) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository budget admission]")
}

// Keep the original cancellation and authorization even when a caller supplies
// a new context. Opaque callbacks run outside SQL; only descriptors enter tx.
func (h repositoryBudgetAdmission) operationContext(ctx context.Context) (context.Context, error) {
	if h.origin == nil {
		return nil, errRepositoryBudgetLease
	}
	if err := checkRepositoryFence(ctx); err != nil {
		return nil, err
	}
	if err := checkRepositoryFence(h.origin); err != nil {
		return nil, err
	}
	return copyRepositoryCredentials(ctx, h.origin), nil
}

func (h repositoryBudgetAdmission) checkOperation(ctx context.Context) error {
	_, err := h.operationContext(ctx)
	return err
}

type repositoryBudgetCharges struct{ api, git, upload, download, requests, cells int64 }
type repositoryBudgetAdmissionRow struct {
	state   string
	charges repositoryBudgetCharges
}

func repositoryBudgetDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func repositoryBudgetUserPrincipal(actor int64) string {
	raw, _ := json.Marshal(struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
		Actor   int64  `json:"actor_id,string"`
	}{1, "user", actor})
	return string(raw)
}

func repositoryBudgetUserRole(purpose string) (string, error) {
	switch purpose {
	case "audit":
		return "operator", nil
	case "scope_preview", "codeowners", "diagnostic":
		return "viewer", nil
	default:
		return "", ErrRepositoryBudget
	}
}

func budgetAccountingError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errRepositoryBudgetAccounting
}

func (s *Store) authorizeBudgetUserTx(ctx context.Context, tx *sql.Tx, h repositoryBudgetAdmission) error {
	role, err := repositoryBudgetUserRole(h.purpose)
	if err != nil || h.actor <= 0 || h.project <= 0 {
		return ErrProjectPermission
	}
	if _, err = requireProjectRole(ctx, tx, h.project, h.actor, role); err != nil {
		if errors.Is(err, ErrProjectPermission) || errors.Is(err, ErrCredentials) || errors.Is(err, sql.ErrNoRows) {
			return ErrProjectPermission
		}
		return budgetAccountingError(ctx)
	}
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, h.project).Scan(&enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProjectPermission
		}
		return budgetAccountingError(ctx)
	}
	if !enabled {
		return ErrProjectPermission
	}
	for _, d := range capturedRepositoryCredentials(ctx) {
		if d.activation == nil || d.activation.store != s || d.validateWithQuery(ctx, tx) != nil {
			return ErrRepositoryUnavailable
		}
	}
	return nil
}

// This is a numeric user-operation budget, never permission to fetch a repo.
// Factory callers must retain their complete repository/credential fences.
func (s *Store) beginUserRepositoryBudget(ctx context.Context, actor int64, project int, purpose string, policy *AuditPolicy) (repositoryBudgetAdmission, error) {
	var h repositoryBudgetAdmission
	if _, err := repositoryBudgetUserRole(purpose); err != nil {
		return h, err
	}
	if err := checkRepositoryFence(ctx); err != nil {
		return h, err
	}
	l, err := repositoryBudgetLimitsFor(policy, purpose)
	if err != nil {
		return h, err
	}
	id, err := newWorkerOwner()
	if err != nil {
		return h, errRepositoryBudgetAccounting
	}
	owner, err := newWorkerOwner()
	if err != nil {
		return h, errRepositoryBudgetAccounting
	}
	principal := repositoryBudgetUserPrincipal(actor)
	limits, _ := json.Marshal(l)
	created := time.Now().UTC()
	h = repositoryBudgetAdmission{store: s, origin: ctx, id: id, owner: owner, purpose: purpose, actor: actor, project: project, principal: principal, principalDigest: repositoryBudgetDigest([]byte(principal)), limitsJSON: string(limits), limitsDigest: repositoryBudgetDigest(limits), limits: *l, deadline: created.Add(time.Duration(l.AdmissionTimeoutSeconds) * time.Second)}
	if policy != nil {
		h.policy = AuditPolicy{TimeoutSeconds: policy.TimeoutSeconds, Git: policy.Git}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return repositoryBudgetAdmission{}, budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	if err = s.authorizeBudgetUserTx(ctx, tx, h); err != nil {
		return repositoryBudgetAdmission{}, err
	}
	if !time.Now().Before(h.deadline) {
		return repositoryBudgetAdmission{}, errRepositoryBudgetLease
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_repository_budgets(id,purpose,project_id,principal_kind,actor_id,principal_json,principal_digest,limits_json,limits_digest,state,session_epoch,admission_owner,admission_lease_until,admission_deadline,api_charged,git_charged,upload_charged,download_charged,requests_charged,match_cells_charged,created_at,updated_at) VALUES(?,?,?,'user',?,?,?,?,?,'admitting',0,?,?,?,0,0,0,0,0,0,?,?)`, h.id, h.purpose, h.project, h.actor, h.principal, h.principalDigest, h.limitsJSON, h.limitsDigest, h.owner, minTime(time.Now().UTC().Add(30*time.Second), h.deadline).Format(time.RFC3339Nano), h.deadline.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano))
	if err != nil {
		return repositoryBudgetAdmission{}, budgetAccountingError(ctx)
	}
	if err = tx.Commit(); err != nil {
		return repositoryBudgetAdmission{}, budgetAccountingError(ctx)
	}
	if err = h.checkOperation(ctx); err != nil {
		return repositoryBudgetAdmission{}, err
	}
	if !time.Now().Before(h.deadline) {
		return repositoryBudgetAdmission{}, errRepositoryBudgetLease
	}
	return h, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *Store) loadBudgetAdmissionTx(ctx context.Context, tx *sql.Tx, h repositoryBudgetAdmission, live bool) (repositoryBudgetAdmissionRow, error) {
	var row repositoryBudgetAdmissionRow
	if h.store != s || h.id == "" || h.owner == "" || h.actor <= 0 || h.project <= 0 || h.limits.validateFor(&h.policy, h.purpose) != nil {
		return row, errRepositoryBudgetLease
	}
	var project int
	var actor, root sql.NullInt64
	var epoch int64
	var purpose, kind, principal, pd, limits, ld, owner, until, deadline, created string
	err := tx.QueryRowContext(ctx, `SELECT project_id,actor_id,root_run_id,purpose,principal_kind,principal_json,principal_digest,limits_json,limits_digest,state,session_epoch,admission_owner,admission_lease_until,admission_deadline,created_at,api_charged,git_charged,upload_charged,download_charged,requests_charged,match_cells_charged FROM platform_repository_budgets WHERE id=?`, h.id).Scan(&project, &actor, &root, &purpose, &kind, &principal, &pd, &limits, &ld, &row.state, &epoch, &owner, &until, &deadline, &created, &row.charges.api, &row.charges.git, &row.charges.upload, &row.charges.download, &row.charges.requests, &row.charges.cells)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, errRepositoryBudgetLease
		}
		return row, budgetAccountingError(ctx)
	}
	if root.Valid || project != h.project || !actor.Valid || actor.Int64 != h.actor || kind != "user" || purpose != h.purpose || epoch != h.epoch || principal != h.principal || pd != h.principalDigest || repositoryBudgetDigest([]byte(principal)) != pd || limits != h.limitsJSON || ld != h.limitsDigest || repositoryBudgetDigest([]byte(limits)) != ld || deadline != h.deadline.Format(time.RFC3339Nano) {
		return row, errRepositoryBudgetLease
	}
	var decoded RepositoryBudgetLimits
	if principal != repositoryBudgetUserPrincipal(h.actor) || json.Unmarshal([]byte(limits), &decoded) != nil || decoded != h.limits {
		return row, errRepositoryBudgetAccounting
	}
	start, err := time.Parse(time.RFC3339Nano, created)
	if err != nil || !start.Add(time.Duration(h.limits.AdmissionTimeoutSeconds)*time.Second).Equal(h.deadline) {
		return row, errRepositoryBudgetAccounting
	}
	for _, p := range [][2]int64{{row.charges.api, h.limits.APIResponseBytes}, {row.charges.git, h.limits.GitResponseBytes}, {row.charges.upload, h.limits.UploadBytes}, {row.charges.download, h.limits.DownloadBytes}, {row.charges.requests, h.limits.Requests}, {row.charges.cells, h.limits.TextMatchCells}} {
		if p[0] < 0 || p[0] > p[1] {
			return row, errRepositoryBudgetAccounting
		}
	}
	if row.charges.download != row.charges.api+row.charges.git {
		return row, errRepositoryBudgetAccounting
	}
	if !live && (row.state == "abandoned" || row.state == "closed") {
		return row, nil
	}
	if row.state != "admitting" || owner != h.owner {
		return row, errRepositoryBudgetLease
	}
	if live {
		lease, err := time.Parse(time.RFC3339Nano, until)
		if err != nil || !time.Now().Before(lease) || !time.Now().Before(h.deadline) || lease.After(h.deadline) {
			return row, errRepositoryBudgetLease
		}
		if err = s.authorizeBudgetUserTx(ctx, tx, h); err != nil {
			return row, err
		}
	}
	return row, nil
}

func (s *Store) renewRepositoryBudgetAdmission(ctx context.Context, h repositoryBudgetAdmission) error {
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
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET admission_lease_until=?,updated_at=? WHERE id=? AND admission_owner=? AND session_epoch=? AND state='admitting' AND julianday(admission_lease_until)>julianday('now') AND julianday(admission_deadline)>julianday('now')`, minTime(time.Now().UTC().Add(30*time.Second), h.deadline).Format(time.RFC3339Nano), now(), h.id, h.owner, h.epoch)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return budgetAccountingError(ctx)
	}
	return h.checkOperation(ctx)
}

// Cleanup revokes this owner's unused authority and never refunds. It remains
// possible after expiry/revocation; it cannot revive or attach an operation.
func (s *Store) endRepositoryBudgetAdmission(ctx context.Context, h repositoryBudgetAdmission) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return budgetAccountingError(ctx)
	}
	defer tx.Rollback()
	row, err := s.loadBudgetAdmissionTx(ctx, tx, h, false)
	if err != nil {
		return err
	}
	if row.state == "closed" || row.state == "abandoned" {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_repository_budget_reservations SET state='charged_unknown',settled_at=? WHERE budget_id=? AND owner_kind='admission' AND owner=? AND lease_epoch=? AND state='open'`, now(), h.id, h.owner, h.epoch); err != nil {
		return budgetAccountingError(ctx)
	}
	state := "closed"
	if h.purpose == "audit" {
		state = "abandoned"
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_budgets SET state=?,admission_owner='',admission_lease_until='',updated_at=? WHERE id=? AND state='admitting' AND admission_owner=? AND session_epoch=?`, state, now(), h.id, h.owner, h.epoch)
	if err = budgetAffectedOne(ctx, result, err); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return budgetAccountingError(ctx)
	}
	return nil
}
