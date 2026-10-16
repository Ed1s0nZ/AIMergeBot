package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func budgetAdmissionFixture(t *testing.T) (*Store, repositoryBudgetAdmission) {
	t.Helper()
	s, admin, _ := accessFixture(t)
	h, err := s.beginUserRepositoryBudget(context.Background(), admin.ID, 1, "audit", repositoryBudgetPolicyFixture())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.endRepositoryBudgetAdmission(context.Background(), h) })
	return s, h
}
func budgetReserve(t *testing.T, s *Store, h repositoryBudgetAdmission, kind string, n int64) (repositoryBudgetReservationKey, repositoryBudgetReceipt) {
	t.Helper()
	k, err := newRepositoryBudgetReservationKey(h, kind, n)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.reserveRepositoryBudget(context.Background(), h, k)
	if err != nil {
		t.Fatal(err)
	}
	return k, r
}
func budgetCharges(t *testing.T, s *Store, h repositoryBudgetAdmission) repositoryBudgetCharges {
	t.Helper()
	var c repositoryBudgetCharges
	if err := s.DB.QueryRow(`SELECT api_charged,git_charged,upload_charged,download_charged,requests_charged,match_cells_charged FROM platform_repository_budgets WHERE id=?`, h.id).Scan(&c.api, &c.git, &c.upload, &c.download, &c.requests, &c.cells); err != nil {
		t.Fatal(err)
	}
	return c
}
func budgetDatabasePath(t *testing.T, s *Store) string {
	t.Helper()
	var seq int
	var name, path string
	if err := s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil || path == "" {
		t.Fatal("file fixture required", err)
	}
	return path
}

func TestRepositoryBudgetAdmissionOwnershipReplayAndSlots(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	ctx := context.Background()
	k, r := budgetReserve(t, s, h, "request", 1)
	again, err := s.reserveRepositoryBudget(ctx, h, k)
	if err != nil || again.id != r.id || !again.replayed || r.replayed || budgetCharges(t, s, h).requests != 1 {
		t.Fatal("duplicate charged or minted twice", err)
	}
	bad := k
	bad.kind = "upload"
	if _, err = s.reserveRepositoryBudget(ctx, h, bad); !errors.Is(err, ErrRepositoryBudget) {
		t.Fatal("operation key inputs changed", err)
	}
	_, _ = budgetReserve(t, s, h, "request", 1)
	third, _ := newRepositoryBudgetReservationKey(h, "request", 1)
	if _, err = s.reserveRepositoryBudget(ctx, h, third); !errors.Is(err, errRepositoryBudgetBusy) || budgetCharges(t, s, h).requests != 2 {
		t.Fatal("slots oversubscribed or charged on wait", err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 0); !errors.Is(err, ErrRepositoryBudget) {
		t.Fatal("request refunded", err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 1); err != nil {
		t.Fatal("same settlement not idempotent", err)
	}
	if _, err = s.reserveRepositoryBudget(ctx, h, third); err != nil || budgetCharges(t, s, h).requests != 3 {
		t.Fatal("closed request did not release slot", err)
	}
	for _, value := range []any{h, k, r} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private handle serialized", err)
		}
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%x"} {
			text := fmt.Sprintf(format, value)
			if strings.Contains(text, h.owner) || strings.Contains(text, h.id) || strings.Contains(text, k.nonce) {
				t.Fatal("private handle leaked")
			}
		}
	}
	if err = s.endRepositoryBudgetAdmission(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err = s.endRepositoryBudgetAdmission(ctx, h); err != nil {
		t.Fatal("close not idempotent", err)
	}
	if _, err = s.reserveRepositoryBudget(ctx, h, k); !errors.Is(err, errRepositoryBudgetLease) {
		t.Fatal("closed budget revived", err)
	}
	if budgetCharges(t, s, h).requests != 3 {
		t.Fatal("close reset consumption")
	}
	var unknown int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_budget_reservations WHERE budget_id=? AND state='charged_unknown'`, h.id).Scan(&unknown); err != nil || unknown != 2 {
		t.Fatal(unknown, err)
	}
}

func TestRepositoryBudgetAdmissionCanonicalReadsRefundAndCells(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	ctx := context.Background()
	k, r := budgetReserve(t, s, h, "api_read", 4<<20)
	if r.reserved != 1<<20 {
		t.Fatal("not a full grant", r.reserved)
	}
	again, err := s.reserveRepositoryBudget(ctx, h, k)
	if err != nil || !again.replayed || again.reserved != r.reserved {
		t.Fatal(err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 123); err != nil {
		t.Fatal(err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 123); err != nil {
		t.Fatal(err)
	}
	if err = s.settleRepositoryBudget(ctx, h, r, 124); !errors.Is(err, ErrRepositoryBudget) {
		t.Fatal("conflicting refund accepted", err)
	}
	c := budgetCharges(t, s, h)
	if c.api != 123 || c.download != 123 {
		t.Fatal(c)
	}
	_, git := budgetReserve(t, s, h, "git_read", 2<<20)
	if err = s.settleRepositoryBudget(ctx, h, git, 0); err != nil {
		t.Fatal(err)
	}
	_, upload := budgetReserve(t, s, h, "upload", 1000)
	if err = s.settleRepositoryBudget(ctx, h, upload, 400); err != nil {
		t.Fatal(err)
	}
	c = budgetCharges(t, s, h)
	if c.git != 0 || c.upload != 400 || c.download != 123 {
		t.Fatal(c)
	}
	for i := 0; i < 8; i++ {
		_, match := budgetReserve(t, s, h, "text_match", 4000000)
		if err = s.settleRepositoryBudget(ctx, h, match, 0); !errors.Is(err, ErrRepositoryBudget) {
			t.Fatal("DP refunded", err)
		}
		if err = s.settleRepositoryBudget(ctx, h, match, match.reserved); err != nil {
			t.Fatal(err)
		}
	}
	ninth, _ := newRepositoryBudgetReservationKey(h, "text_match", 1)
	if _, err = s.reserveRepositoryBudget(ctx, h, ninth); !errors.Is(err, errRepositoryBudgetExhausted) || budgetCharges(t, s, h).cells != 32000000 {
		t.Fatal("matching allowance reset", err)
	}
	// Last category bytes are a smaller canonical grant, not a new allowance.
	if _, err = s.DB.Exec(`UPDATE platform_repository_budgets SET api_charged=?,download_charged=? WHERE id=?`, h.limits.APIResponseBytes-7, h.limits.APIResponseBytes-7, h.id); err != nil {
		t.Fatal(err)
	}
	_, tail := budgetReserve(t, s, h, "api_read", 4<<20)
	if tail.reserved != 7 {
		t.Fatal(tail.reserved)
	}
	last, _ := newRepositoryBudgetReservationKey(h, "api_read", 1)
	if _, err = s.reserveRepositoryBudget(ctx, h, last); !errors.Is(err, errRepositoryBudgetExhausted) {
		t.Fatal("exhaustion became free EOF probe", err)
	}
}

func TestRepositoryBudgetAdmissionConcurrentLastRequestAndRefund(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	ctx := context.Background()
	other, err := OpenStore(budgetDatabasePath(t, s))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	second := h
	second.store = other // Same in-process capability, independent SQL connection.
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_repository_budgets SET requests_charged=511 WHERE id=?`, h.id); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<511) INSERT INTO platform_repository_budget_reservations(id,budget_id,owner_kind,owner,lease_epoch,kind,reserved,state,settled_used,created_at,settled_at) SELECT printf('%064x',x),?,'admission',?,0,'request',1,'settled',1,?,? FROM n`, h.id, h.owner, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, handle := s, h
			if i%2 == 1 {
				store, handle = other, second
			}
			key, err := newRepositoryBudgetReservationKey(handle, "request", 1)
			if err == nil {
				_, err = store.reserveRepositoryBudget(ctx, handle, key)
			}
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, errRepositoryBudgetExhausted) {
			t.Fatal(err)
		}
	}
	if success != 1 || budgetCharges(t, s, h).requests != 512 {
		t.Fatal("concurrent overspend", success)
	}
	_, grant := budgetReserve(t, s, h, "api_read", 2<<20)
	results = make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, handle := s, h
			if i%2 == 1 {
				store, handle = other, second
			}
			results <- store.settleRepositoryBudget(ctx, handle, grant, 77)
		}(i)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	c := budgetCharges(t, s, h)
	if c.api != 77 || c.download != 77 || c.requests != 512 {
		t.Fatal("duplicate refund", c)
	}
}

func TestRepositoryBudgetAdmissionLeaseRevocationAndUnknown(t *testing.T) {
	for _, mode := range []string{"expired", "owner", "epoch", "deadline", "disabled_actor", "disabled_project", "context_fence"} {
		t.Run(mode, func(t *testing.T) {
			s, h := budgetAdmissionFixture(t)
			key, grant := budgetReserve(t, s, h, "api_read", 2<<20)
			ctx := context.Background()
			var err error
			switch mode {
			case "expired":
				_, err = s.DB.Exec(`UPDATE platform_repository_budgets SET admission_lease_until=? WHERE id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), h.id)
			case "owner":
				_, err = s.DB.Exec(`UPDATE platform_repository_budgets SET admission_owner=? WHERE id=?`, strings.Repeat("a", 64), h.id)
			case "epoch":
				_, err = s.DB.Exec(`UPDATE platform_repository_budgets SET session_epoch=session_epoch+1 WHERE id=?`, h.id)
			case "deadline":
				_, err = s.DB.Exec(`UPDATE platform_repository_budgets SET admission_deadline=? WHERE id=?`, h.deadline.Add(time.Hour).Format(time.RFC3339Nano), h.id)
			case "disabled_actor":
				_, err = s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, h.actor)
			case "disabled_project":
				_, err = s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=?`, h.project)
			case "context_fence":
				ctx = withRepositoryFence(ctx, func(context.Context) error { return errors.New("PRIVATE") })
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.reserveRepositoryBudget(ctx, h, key); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("old authority continued", err)
			}
			if err = s.renewRepositoryBudgetAdmission(ctx, h); err == nil {
				t.Fatal("stale owner renewed")
			}
			if err = s.settleRepositoryBudget(ctx, h, grant, 0); err == nil {
				t.Fatal("stale owner refunded")
			}
			if budgetCharges(t, s, h).api != 1<<20 {
				t.Fatal("unknown consumption erased")
			}
			if mode == "expired" || mode == "disabled_actor" || mode == "disabled_project" || mode == "context_fence" {
				if err = s.endRepositoryBudgetAdmission(context.Background(), h); err != nil {
					t.Fatal("safe cleanup blocked", err)
				}
			}
		})
	}
}

func TestRepositoryBudgetAdmissionReopenAndFenceNoDeadlock(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	key, grant := budgetReserve(t, s, h, "api_read", 2<<20)
	path := budgetDatabasePath(t, s)
	s.Close()
	other, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	retained := h
	retained.store = other // Reopen connection only: no public recovery by ID.
	if budgetCharges(t, other, retained).api != 1<<20 {
		t.Fatal("reopen reset charges")
	}
	if r, err := other.reserveRepositoryBudget(context.Background(), retained, key); err != nil || !r.replayed {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ctx = withRepositoryFence(ctx, func(ctx context.Context) error {
		var n int
		return other.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_projects`).Scan(&n)
	})
	if err = other.settleRepositoryBudget(ctx, retained, grant, 3); err != nil {
		t.Fatal("fence called under SQL lock", err)
	}
	if err = other.renewRepositoryBudgetAdmission(ctx, retained); err != nil {
		t.Fatal(err)
	}
	var deadline string
	if err = other.DB.QueryRow(`SELECT admission_deadline FROM platform_repository_budgets WHERE id=?`, h.id).Scan(&deadline); err != nil || deadline != h.deadline.Format(time.RFC3339Nano) {
		t.Fatal("renew extended absolute deadline", err)
	}
	if err = other.endRepositoryBudgetAdmission(context.Background(), retained); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryBudgetAdmissionFailureRollbackAndReservationCap(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	ctx := context.Background()
	key, _ := newRepositoryBudgetReservationKey(h, "request", 1)
	if _, err := s.DB.Exec(`CREATE TRIGGER synthetic_budget_fail BEFORE INSERT ON platform_repository_budget_reservations BEGIN SELECT RAISE(ABORT,'PRIVATE'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.reserveRepositoryBudget(ctx, h, key); !errors.Is(err, errRepositoryBudgetAccounting) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal(err)
	}
	if budgetCharges(t, s, h).requests != 0 {
		t.Fatal("failed receipt left charge")
	}
	if _, err := s.DB.Exec(`DROP TRIGGER synthetic_budget_fail`); err != nil {
		t.Fatal(err)
	}
	_, grant := budgetReserve(t, s, h, "api_read", 1<<20)
	if _, err := s.DB.Exec(`CREATE TRIGGER synthetic_budget_fail BEFORE UPDATE ON platform_repository_budget_reservations WHEN NEW.state='settled' BEGIN SELECT RAISE(ABORT,'PRIVATE'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.settleRepositoryBudget(ctx, h, grant, 0); !errors.Is(err, errRepositoryBudgetAccounting) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal(err)
	}
	if budgetCharges(t, s, h).api != 1<<20 {
		t.Fatal("failed settlement left refund")
	}
	if _, err := s.DB.Exec(`DROP TRIGGER synthetic_budget_fail`); err != nil {
		t.Fatal(err)
	}
	if err := s.settleRepositoryBudget(ctx, h, grant, 0); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_repository_budgets SET api_charged=8191,download_charged=8191 WHERE id=?`, h.id); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<8191) INSERT INTO platform_repository_budget_reservations(id,budget_id,owner_kind,owner,lease_epoch,kind,reserved,state,created_at,settled_at) SELECT printf('%064x',x),?,'admission',?,0,'api_read',1,'charged_unknown',?,? FROM n`, h.id, h.owner, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.reserveRepositoryBudget(ctx, h, key); !errors.Is(err, errRepositoryBudgetExhausted) || budgetCharges(t, s, h).requests != 0 {
		t.Fatal("8192 receipt cap bypassed", err)
	}
}

func TestRepositoryBudgetAdmissionRejectsUnprovenPrincipal(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		actor   int64
		purpose string
	}{{0, "audit"}, {member.ID, "audit"}, {admin.ID, "poll"}, {admin.ID, "publication"}} {
		if _, err := s.beginUserRepositoryBudget(ctx, tc.actor, 1, tc.purpose, repositoryBudgetPolicyFixture()); err == nil {
			t.Fatal("unproven principal admitted", tc)
		}
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_budgets`).Scan(&n); err != nil || n != 0 {
		t.Fatal("rejected operation persisted", n, err)
	}
}

func TestRepositoryBudgetAdmissionCapturedCredentialsUseTransaction(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{})
	admin, err := s.CreateUser(context.Background(), "budget-admin", "long-synthetic-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(context.Background(), Project{ID: 1, Name: "budget", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx, _, err = svc.legacyOperationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx = withRepositoryFence(ctx, func(ctx context.Context) error {
		var n int
		return s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_projects`).Scan(&n)
	})
	h, err := s.beginUserRepositoryBudget(ctx, admin.ID, 1, "audit", repositoryBudgetPolicyFixture())
	if err != nil {
		t.Fatal("transaction reentered Store.DB", err)
	}
	key, err := newRepositoryBudgetReservationKey(h, "api_read", 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.reserveRepositoryBudget(ctx, h, key); err != nil {
		t.Fatal(err)
	}
	next := svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(next); err != nil {
		t.Fatal(err)
	}
	if _, err = s.reserveRepositoryBudget(ctx, h, key); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("captured generation replaced", err)
	}
	if budgetCharges(t, s, h).api != 1<<20 {
		t.Fatal("credential change reset charges")
	}
	if err = s.endRepositoryBudgetAdmission(context.Background(), h); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryBudgetAdmissionPostCommitFenceKeepsCharge(t *testing.T) {
	s, h := budgetAdmissionFixture(t)
	key, err := newRepositoryBudgetReservationKey(h, "request", 1)
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	ctx := withRepositoryFence(context.Background(), func(context.Context) error {
		checks++
		if checks == 2 {
			return errors.New("PRIVATE")
		}
		return nil
	})
	if _, err = s.reserveRepositoryBudget(ctx, h, key); !errors.Is(err, ErrRepositoryUnavailable) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal(err)
	}
	if budgetCharges(t, s, h).requests != 1 {
		t.Fatal("committed reservation was erased")
	}
	replay, err := s.reserveRepositoryBudget(context.Background(), h, key)
	if err != nil || !replay.replayed || budgetCharges(t, s, h).requests != 1 {
		t.Fatal("uncertain reply minted a second permit", err)
	}
}

func TestRepositoryBudgetAdmissionOriginalFenceCannotBeDropped(t *testing.T) {
	s, admin, _ := accessFixture(t)
	blocked := false
	original := withRepositoryFence(context.Background(), func(context.Context) error {
		if blocked {
			return errors.New("PRIVATE")
		}
		return nil
	})
	h, err := s.beginUserRepositoryBudget(original, admin.ID, 1, "audit", repositoryBudgetPolicyFixture())
	if err != nil {
		t.Fatal(err)
	}
	key, err := newRepositoryBudgetReservationKey(h, "request", 1)
	if err != nil {
		t.Fatal(err)
	}
	blocked = true
	if _, err = s.reserveRepositoryBudget(context.Background(), h, key); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("background context discarded original fence", err)
	}
	if budgetCharges(t, s, h).requests != 0 {
		t.Fatal("discarded fence charged a new operation")
	}
	if err = s.endRepositoryBudgetAdmission(context.Background(), h); err != nil {
		t.Fatal("cleanup inherited obsolete read permission", err)
	}
}
