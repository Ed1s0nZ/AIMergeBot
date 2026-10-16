package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func budgetRunFixture(t *testing.T) (*Store, repositoryBudgetAdmission, Snapshot) {
	t.Helper()
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	b, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example"))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := readRepositoryIdentity(ctx, s.DB, 1)
	if err != nil {
		t.Fatal(err)
	}
	scope := repositoryScopeFixture()
	scope.Version = 2
	scope.Target = RepositoryReference{ProjectID: 1, IdentityRevision: identity.Revision, Provider: b.Provider, APIOrigin: b.APIOrigin, RemoteID: b.RemoteID, FullName: b.FullName, BindingRevision: b.Revision, Credential: RepositoryCredential{Kind: "integration", IntegrationID: v.ID, IntegrationRevision: v.Revision}}
	scope.Source = scope.Target
	snap := repositoryScopeSnapshot(scope)
	p := repositoryBudgetPolicyFixture()
	p.RepositoryScope = scope
	p.RepositoryBudget, err = repositoryBudgetLimitsFor(p, "audit")
	if err != nil {
		t.Fatal(err)
	}
	snap.AuditPolicy = p
	h, err := s.beginUserRepositoryBudget(ctx, admin.ID, 1, "audit", p)
	if err != nil {
		t.Fatal(err)
	}
	return s, h, snap
}

func attachBudgetRun(t *testing.T, s *Store, h repositoryBudgetAdmission, snap Snapshot) int64 {
	t.Helper()
	id, created, err := s.enqueueRepositoryBudgetRun(context.Background(), h, snap, false)
	if err != nil || !created {
		t.Fatal("attach", id, created, err)
	}
	return id
}

func budgetRunWorker(t *testing.T, s *Store, id int64) string {
	t.Helper()
	ctx := context.Background()
	owner, err := newWorkerOwner()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkerInstance(ctx, owner); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimOwned(ctx, owner)
	if err != nil || claimed != id {
		t.Fatal("claim", claimed, err)
	}
	return owner
}

func TestRepositoryBudgetRunAttachAndDuplicatePreserveCharges(t *testing.T) {
	s, h, snap := budgetRunFixture(t)
	ctx := context.Background()
	_, receipt := budgetReserve(t, s, h, "api_read", 100)
	id := attachBudgetRun(t, s, h, snap)
	var state, mapped, receiptState string
	var root int64
	if err := s.DB.QueryRow(`SELECT state,root_run_id FROM platform_repository_budgets WHERE id=?`, h.id).Scan(&state, &root); err != nil || state != "active" || root != id {
		t.Fatal(state, root, err)
	}
	if err := s.DB.QueryRow(`SELECT budget_id FROM platform_run_repository_budgets WHERE run_id=?`, id).Scan(&mapped); err != nil || mapped != h.id {
		t.Fatal("mapping", err)
	}
	if err := s.DB.QueryRow(`SELECT state FROM platform_repository_budget_reservations WHERE id=?`, receipt.id).Scan(&receiptState); err != nil || receiptState != "charged_unknown" {
		t.Fatal("open receipt survived attach", err)
	}
	if err := s.settleRepositoryBudget(ctx, h, receipt, 0); err == nil || budgetCharges(t, s, h).api != 100 {
		t.Fatal("admission refunded after attach", err)
	}
	second, err := s.beginUserRepositoryBudget(ctx, h.actor, 1, "audit", snap.AuditPolicy)
	if err != nil {
		t.Fatal(err)
	}
	budgetReserve(t, s, second, "request", 1)
	got, created, err := s.enqueueRepositoryBudgetRun(ctx, second, snap, false)
	if err != nil || created || got != id {
		t.Fatal("duplicate", got, created, err)
	}
	if err = s.DB.QueryRow(`SELECT state FROM platform_repository_budgets WHERE id=?`, second.id).Scan(&state); err != nil || state != "abandoned" || budgetCharges(t, s, second).requests != 1 || budgetCharges(t, s, h).api != 100 {
		t.Fatal("duplicate reopened budget", state, err)
	}
}

func TestRepositoryBudgetRunAttachRollback(t *testing.T) {
	for _, table := range []string{"platform_run_repository_budgets", "platform_events"} {
		t.Run(table, func(t *testing.T) {
			s, h, snap := budgetRunFixture(t)
			budgetReserve(t, s, h, "request", 1)
			if _, err := s.DB.Exec(`CREATE TRIGGER reject_budget_attach BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'PRIVATE'); END`); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.enqueueRepositoryBudgetRun(context.Background(), h, snap, false); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("rollback/redaction", err)
			}
			var runs, mappings, open int
			var state string
			if err := s.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM platform_runs),(SELECT COUNT(*) FROM platform_run_repository_budgets),(SELECT COUNT(*) FROM platform_repository_budget_reservations WHERE state='open')`).Scan(&runs, &mappings, &open); err != nil {
				t.Fatal(err)
			}
			if err := s.DB.QueryRow(`SELECT state FROM platform_repository_budgets WHERE id=?`, h.id).Scan(&state); err != nil || runs != 0 || mappings != 0 || open != 1 || state != "admitting" || budgetCharges(t, s, h).requests != 1 {
				t.Fatal("partial attach", runs, mappings, open, state, err)
			}
		})
	}
}

func TestRepositoryBudgetRunDuplicateRejectsForeignRoot(t *testing.T) {
	s, h, snap := budgetRunFixture(t)
	ctx := context.Background()
	id := attachBudgetRun(t, s, h, snap)
	other := snap
	p := *snap.AuditPolicy
	p.RepositoryScope = p.RepositoryScope.clone()
	p.RepositoryScope.Observation.Number++
	other.MRIID++
	other.AuditPolicy = &p
	h2, err := s.beginUserRepositoryBudget(ctx, h.actor, 1, "audit", other.AuditPolicy)
	if err != nil {
		t.Fatal(err)
	}
	attachBudgetRun(t, s, h2, other)
	if _, err = s.DB.Exec(`UPDATE platform_run_repository_budgets SET budget_id=? WHERE run_id=?`, h2.id, id); err != nil {
		t.Fatal(err)
	}
	h3, err := s.beginUserRepositoryBudget(ctx, h.actor, 1, "audit", snap.AuditPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.enqueueRepositoryBudgetRun(ctx, h3, snap, false); err == nil {
		t.Fatal("duplicate accepted a foreign budget root")
	}
	var state string
	if err = s.DB.QueryRow(`SELECT state FROM platform_repository_budgets WHERE id=?`, h3.id).Scan(&state); err != nil || state != "admitting" {
		t.Fatal("rejected duplicate ended admission", state, err)
	}
}

func TestRepositoryBudgetWorkerEpochAndRevocation(t *testing.T) {
	s, h, snap := budgetRunFixture(t)
	ctx := context.Background()
	budgetReserve(t, s, h, "request", 1)
	id := attachBudgetRun(t, s, h, snap)
	owner := budgetRunWorker(t, s, id)
	w1, err := s.openRepositoryBudgetWorker(ctx, id, owner)
	if err != nil || w1.epoch != 1 {
		t.Fatal("first epoch", err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w1); err != nil {
		t.Fatal(err)
	}
	w2, err := s.openRepositoryBudgetWorker(ctx, id, owner)
	if err != nil || w2.epoch != 2 {
		t.Fatal("second epoch", err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w1); !errors.Is(err, errRepositoryBudgetLease) {
		t.Fatal("old session survived", err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w2); err != nil || budgetCharges(t, s, h).requests != 1 {
		t.Fatal("new session reset", err)
	}
	if _, err = s.openRepositoryBudgetWorker(ctx, id, "wrong"); !errors.Is(err, ErrWorkerLeaseLost) {
		t.Fatal("wrong owner", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_integrations SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w2); err == nil {
		t.Fatal("revoked credential accepted")
	}
}

func TestRepositoryBudgetRunAuthorizationDeniesBeforeWrite(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE platform_projects SET enabled=0 WHERE id=1`,
		`UPDATE platform_integrations SET enabled=0`,
		`UPDATE platform_integrations SET revision=revision+1`,
		`UPDATE platform_repository_identities SET revision=revision+1 WHERE project_id=1`,
		`UPDATE platform_users SET disabled=1`,
	} {
		t.Run(mutation, func(t *testing.T) {
			s, h, snap := budgetRunFixture(t)
			if _, err := s.DB.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.enqueueRepositoryBudgetRun(context.Background(), h, snap, false); err == nil {
				t.Fatal("revoked enqueue allowed")
			}
			var n int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&n); err != nil || n != 0 {
				t.Fatal("revoked enqueue wrote", n, err)
			}
		})
	}
}

func TestRepositoryBudgetRunChecksSourceAndContextIndex(t *testing.T) {
	for _, mode := range []string{"source_revoked", "context_revoked", "context_index"} {
		t.Run(mode, func(t *testing.T) {
			s, h, snap := budgetRunFixture(t)
			ctx := context.Background()
			v := bindingIntegration(t, s, h.actor, "gitlab", "https://gitlab.example", 2)
			binding := bindingValue(v, "gitlab", "https://gitlab.example")
			if mode == "source_revoked" {
				v = Integration{ID: snap.AuditPolicy.RepositoryScope.Target.Credential.IntegrationID, Revision: snap.AuditPolicy.RepositoryScope.Target.Credential.IntegrationRevision}
				binding = bindingValue(v, "github", "https://api.example")
				binding.RemoteID = 8
				binding.FullName = "fork/repo"
			}
			b, err := s.SaveRepositoryBinding(ctx, 2, h.actor, 0, binding)
			if err != nil {
				t.Fatal(err)
			}
			identity, err := readRepositoryIdentity(ctx, s.DB, 2)
			if err != nil {
				t.Fatal(err)
			}
			ref := RepositoryReference{ProjectID: 2, IdentityRevision: identity.Revision, Provider: b.Provider, APIOrigin: b.APIOrigin, RemoteID: b.RemoteID, FullName: b.FullName, BindingRevision: b.Revision, Credential: RepositoryCredential{Kind: "integration", IntegrationID: v.ID, IntegrationRevision: v.Revision}}
			if mode == "source_revoked" {
				snap.SourceProjectID = 2
				snap.AuditPolicy.RepositoryScope.Source = ref
			} else {
				sha := strings.Repeat("d", 40)
				snap.AuditPolicy.RepositoryScope.Context = []RepositoryContextReference{{Repository: ref, SHA: sha}}
				snap.AuditPolicy.ContextRepositories = []ContextRepository{{ProjectID: 2, SHA: sha}}
			}
			if err = s.authorizeRepositoryBudgetSnapshot(ctx, s.DB, snap, h.actor); err != nil {
				t.Fatal("fixture must be authorized before revocation", err)
			}
			if mode != "context_index" {
				if _, err = s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=2`); err != nil {
					t.Fatal(err)
				}
				if _, _, err = s.enqueueRepositoryBudgetRun(ctx, h, snap, false); err == nil {
					t.Fatal("revoked related repository accepted")
				}
				return
			}
			id := attachBudgetRun(t, s, h, snap)
			owner := budgetRunWorker(t, s, id)
			w, err := s.openRepositoryBudgetWorker(ctx, id, owner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec(`DELETE FROM platform_run_context_repositories WHERE run_id=?`, id); err != nil {
				t.Fatal(err)
			}
			if err = s.validateRepositoryBudgetWorker(ctx, w); err == nil {
				t.Fatal("missing context index accepted")
			}
		})
	}
}

// This fixture models the caller's single transaction; it is not an automatic
// retry integration test. N1d2 must exercise the real scheduler separately.
func insertBudgetRetryFixture(ctx context.Context, tx *sql.Tx, parent int64) (int64, error) {
	if _, err := tx.ExecContext(ctx, `UPDATE platform_runs SET status='failed' WHERE id=?`, parent); err != nil {
		return 0, err
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version,policy_digest,audit_policy_json,retry_parent_id,retry_attempt) SELECT project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,'pending',?,requested_by,policy_version,policy_digest,audit_policy_json,id,retry_attempt+1 FROM platform_runs WHERE id=?`, now(), parent)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func TestRepositoryBudgetRetryInheritsRootWithoutReset(t *testing.T) {
	s, h, snap := budgetRunFixture(t)
	ctx := context.Background()
	budgetReserve(t, s, h, "api_read", 91)
	parent := attachBudgetRun(t, s, h, snap)
	owner := budgetRunWorker(t, s, parent)
	w, err := s.openRepositoryBudgetWorker(ctx, parent, owner)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	child, err := insertBudgetRetryFixture(ctx, tx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.inheritRepositoryRetryBudgetTx(ctx, tx, parent, child); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var mapped string
	if err = s.DB.QueryRow(`SELECT budget_id FROM platform_run_repository_budgets WHERE run_id=?`, child).Scan(&mapped); err != nil || mapped != h.id || budgetCharges(t, s, h).api != 91 {
		t.Fatal("child received fresh budget", err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w); err == nil {
		t.Fatal("parent worker survived retry")
	}
	if got, err := s.ClaimOwned(ctx, owner); err != nil || got != child {
		t.Fatal(got, err)
	}
	w2, err := s.openRepositoryBudgetWorker(ctx, child, owner)
	if err != nil || w2.epoch != 3 {
		t.Fatal("child epoch", err)
	}
	if err = s.validateRepositoryBudgetWorker(ctx, w2); err != nil {
		t.Fatal(err)
	}
	// A stale ancestor cannot reopen the ledger even if its running flag was
	// incorrectly restored after the child finished.
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='failed' WHERE id=?`, child); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='running' WHERE id=?`, parent); err != nil {
		t.Fatal(err)
	}
	if _, err = s.openRepositoryBudgetWorker(ctx, parent, owner); err == nil {
		t.Fatal("non-tail attempt reopened ledger")
	}
}

func TestRepositoryBudgetRetryRejectsAndRollsBack(t *testing.T) {
	for _, mode := range []string{"insert_failure", "changed_head", "parent_cancelled", "child_running"} {
		t.Run(mode, func(t *testing.T) {
			s, h, snap := budgetRunFixture(t)
			ctx := context.Background()
			parent := attachBudgetRun(t, s, h, snap)
			owner := budgetRunWorker(t, s, parent)
			w, err := s.openRepositoryBudgetWorker(ctx, parent, owner)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "insert_failure" {
				if _, err = s.DB.Exec(`CREATE TRIGGER reject_retry_map BEFORE INSERT ON platform_run_repository_budgets BEGIN SELECT RAISE(ABORT,'PRIVATE'); END`); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			child, err := insertBudgetRetryFixture(ctx, tx, parent)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "changed_head":
				_, err = tx.Exec(`UPDATE platform_runs SET head_sha=? WHERE id=?`, strings.Repeat("d", 40), child)
			case "parent_cancelled":
				_, err = tx.Exec(`UPDATE platform_runs SET status='cancelled' WHERE id=?`, parent)
			case "child_running":
				_, err = tx.Exec(`UPDATE platform_runs SET status='running' WHERE id=?`, child)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.inheritRepositoryRetryBudgetTx(ctx, tx, parent, child); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("invalid retry mapping", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			var n int
			var state string
			if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&n); err != nil || n != 1 {
				t.Fatal("orphan retry", n, err)
			}
			if err = s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, parent).Scan(&state); err != nil || state != "running" {
				t.Fatal("parent status not rolled back", state, err)
			}
			if err = s.validateRepositoryBudgetWorker(ctx, w); err != nil {
				t.Fatal("rollback revoked original worker", err)
			}
		})
	}
}

func TestRepositoryBudgetClosedDuplicateCannotOpenWorker(t *testing.T) {
	s, h, snap := budgetRunFixture(t)
	ctx := context.Background()
	id := attachBudgetRun(t, s, h, snap)
	owner := budgetRunWorker(t, s, id)
	if _, err := s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_repository_budgets SET state='closed' WHERE id=?`, h.id); err != nil {
		t.Fatal(err)
	}
	h2, err := s.beginUserRepositoryBudget(ctx, h.actor, 1, "audit", snap.AuditPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if got, created, err := s.enqueueRepositoryBudgetRun(ctx, h2, snap, false); err != nil || created || got != id {
		t.Fatal("closed duplicate", got, created, err)
	}
	// Even a damaged running status cannot reopen a closed root.
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='running' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.openRepositoryBudgetWorker(ctx, id, owner); err == nil {
		t.Fatal("closed root reopened")
	}
}

func TestRepositoryBudgetWorkerOriginLeaseAndReopen(t *testing.T) {
	for _, mode := range []string{"origin_cancel", "run_expired", "instance_expired", "reopen"} {
		t.Run(mode, func(t *testing.T) {
			s, h, snap := budgetRunFixture(t)
			ctx := context.Background()
			budgetReserve(t, s, h, "request", 1)
			id := attachBudgetRun(t, s, h, snap)
			owner := budgetRunWorker(t, s, id)
			origin, cancel := context.WithCancel(ctx)
			defer cancel()
			w, err := s.openRepositoryBudgetWorker(origin, id, owner)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "origin_cancel":
				cancel()
			case "run_expired":
				_, err = s.DB.Exec(`UPDATE platform_runs SET worker_lease_until='2000-01-01T00:00:00Z' WHERE id=?`, id)
			case "instance_expired":
				_, err = s.DB.Exec(`UPDATE platform_worker_instance SET lease_until='2000-01-01T00:00:00Z'`)
			case "reopen":
				other, e := OpenStore(budgetDatabasePath(t, s))
				if e != nil {
					t.Fatal(e)
				}
				defer other.Close()
				fresh, e := other.openRepositoryBudgetWorker(ctx, id, owner)
				if e != nil || fresh.epoch != 2 {
					t.Fatal("reopened worker", e)
				}
				if e = other.validateRepositoryBudgetWorker(ctx, fresh); e != nil {
					t.Fatal(e)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = s.validateRepositoryBudgetWorker(ctx, w); err == nil {
				t.Fatal(fmt.Sprintf("old authority survived %s", mode))
			}
			if budgetCharges(t, s, h).requests != 1 {
				t.Fatal("worker failure reset charges")
			}
		})
	}
}
