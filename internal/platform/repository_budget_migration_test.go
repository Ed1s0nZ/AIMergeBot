package platform

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Recreate the previous release's schema2 shape, then seed state that the
// budget upgrade must not regenerate or reinterpret.
func previousBudgetDatabase(t *testing.T) (string, *Store) {
	t.Helper()
	path, _ := oldIdentityDatabase(t)
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, q := range []string{`DROP TABLE platform_repository_budget_reservations`, `DROP TABLE platform_run_repository_budgets`, `DROP TABLE platform_repository_budgets`, `UPDATE platform_schema SET version=2`, `UPDATE platform_legacy_repository_state SET generation=7,fingerprint=zeroblob(32)`, `UPDATE platform_repository_identities SET revision=9 WHERE project_id=1`, `UPDATE platform_runs SET status='pending',error='preserve-exactly' WHERE id=1`} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return path, s
}

func TestRepositoryBudgetSchemaUpgradePreservesIdentityAndHistory(t *testing.T) {
	path, old := previousBudgetDatabase(t)
	var keyBefore, fpBefore []byte
	if err := old.DB.QueryRow(`SELECT hmac_key,fingerprint FROM platform_legacy_repository_state WHERE id=1`).Scan(&keyBefore, &fpBefore); err != nil {
		t.Fatal(err)
	}
	var policyBefore, resultBefore string
	if err := old.DB.QueryRow(`SELECT audit_policy_json,result_json FROM platform_runs WHERE id=1`).Scan(&policyBefore, &resultBefore); err != nil {
		t.Fatal(err)
	}
	old.Close()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var version, generation, revision int
	var key, fp []byte
	if err = s.DB.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 3 {
		t.Fatal(version, err)
	}
	if err = s.DB.QueryRow(`SELECT generation,hmac_key,fingerprint FROM platform_legacy_repository_state WHERE id=1`).Scan(&generation, &key, &fp); err != nil || generation != 7 || !reflect.DeepEqual(key, keyBefore) || !reflect.DeepEqual(fp, fpBefore) {
		t.Fatal("identity state rotated", err)
	}
	if err = s.DB.QueryRow(`SELECT revision FROM platform_repository_identities WHERE project_id=1`).Scan(&revision); err != nil || revision != 9 {
		t.Fatal("identity registration recreated", err)
	}
	var status, errorText, policy, result string
	if err = s.DB.QueryRow(`SELECT status,error,audit_policy_json,result_json FROM platform_runs WHERE id=1`).Scan(&status, &errorText, &policy, &result); err != nil || status != "pending" || errorText != "preserve-exactly" || policy != policyBefore || result != resultBefore {
		t.Fatal("history reinterpreted", err)
	}
	for table := range repositoryBudgetColumns {
		var n int
		if err = s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("old run given invented budget", table, err)
		}
	}
	manifest := schemaManifest(t, s.DB)
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !reflect.DeepEqual(manifest, schemaManifest(t, s.DB)) {
		t.Fatal("reopen changed schema")
	}
}

func TestRepositoryBudgetSchemaStoppedUpgradeAndAtomicRollback(t *testing.T) {
	for _, mode := range []string{"active_worker", "namespace_collision"} {
		t.Run(mode, func(t *testing.T) {
			path, s := previousBudgetDatabase(t)
			if mode == "active_worker" {
				_, err := s.DB.Exec(`INSERT OR REPLACE INTO platform_worker_instance(id,owner,lease_until) VALUES(1,?,?)`, strings.Repeat("a", 64), time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.DB.Exec(`CREATE TABLE platform_repository_budgets(sentinel TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			before := schemaManifest(t, s.DB)
			opened, err := OpenStore(path)
			if opened != nil {
				opened.Close()
			}
			if err == nil || mode == "active_worker" && !errors.Is(err, ErrWorkerInstanceActive) {
				t.Fatal("unsafe upgrade admitted", err)
			}
			if !reflect.DeepEqual(before, schemaManifest(t, s.DB)) {
				t.Fatal("failed upgrade left partial schema")
			}
			var version, generation int
			if err = s.DB.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 2 {
				t.Fatal(version, err)
			}
			if err = s.DB.QueryRow(`SELECT generation FROM platform_legacy_repository_state`).Scan(&generation); err != nil || generation != 7 {
				t.Fatal("failed upgrade rotated state", err)
			}
		})
	}
}

func TestRepositoryBudgetSchemaBrokenCurrentIsNeverRecreated(t *testing.T) {
	for _, mode := range []string{"missing_table", "missing_column", "unversioned_future"} {
		t.Run(mode, func(t *testing.T) {
			path, s := previousBudgetDatabase(t)
			s.Close()
			s, err := OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			switch mode {
			case "missing_table":
				_, err = s.DB.Exec(`DROP TABLE platform_repository_budget_reservations`)
			case "missing_column":
				_, err = s.DB.Exec(`ALTER TABLE platform_repository_budget_reservations RENAME COLUMN reserved TO unexpected`)
			case "unversioned_future":
				_, err = s.DB.Exec(`DROP TABLE platform_schema`)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := schemaManifest(t, s.DB)
			other, err := OpenStore(path)
			if other != nil {
				other.Close()
			}
			if err == nil {
				t.Fatal("broken schema repaired itself")
			}
			if !reflect.DeepEqual(before, schemaManifest(t, s.DB)) {
				t.Fatal("broken schema mutated")
			}
		})
	}
}

func TestRepositoryBudgetSchemaAccountingAndReceiptConstraints(t *testing.T) {
	s, admin, _ := accessFixture(t)
	l, err := repositoryBudgetLimitsFor(repositoryBudgetPolicyFixture(), "audit")
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := json.Marshal(l)
	id, owner, hash := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	created := time.Now().UTC()
	_, err = s.DB.Exec(`INSERT INTO platform_repository_budgets(id,purpose,project_id,principal_kind,actor_id,principal_json,principal_digest,limits_json,limits_digest,state,session_epoch,admission_owner,admission_lease_until,admission_deadline,api_charged,git_charged,upload_charged,download_charged,requests_charged,match_cells_charged,created_at,updated_at) VALUES(?,'audit',1,'user',?,'{}',?,? ,?,'admitting',0,?,?,?,0,0,0,0,0,0,?,?)`, id, admin.ID, hash, string(limits), hash, owner, created.Add(30*time.Second).Format(time.RFC3339Nano), created.Add(time.Minute).Format(time.RFC3339Nano), created.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	for name, update := range map[string]string{
		"negative": `requests_charged=-1`, "fractional": `requests_charged=0.5`, "over_limit": `requests_charged=513`,
		"category_limit": `api_charged=33554433,download_charged=33554433`, "sum_mismatch": `download_charged=1`,
		"missing_limit": `limits_json=json_remove(limits_json,'$.requests')`, "null_limit": `limits_json=json_set(limits_json,'$.requests',NULL)`, "string_limit": `limits_json=json_set(limits_json,'$.requests','512')`,
		"user_without_actor": `actor_id=NULL`, "fake_actor_zero": `actor_id=0`, "service_with_actor": `principal_kind='service'`,
		"active_without_run": `state='active',admission_owner='',admission_lease_until=''`, "seed_without_digest": `admission_seed_json='{}'`, "lease_after_deadline": `admission_lease_until='2999-01-01T00:00:00Z'`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.DB.Exec("UPDATE platform_repository_budgets SET "+update+" WHERE id=?", id); err == nil {
				t.Fatal("invalid accounting accepted")
			}
		})
	}
	if _, err = s.DB.Exec(`UPDATE platform_repository_budgets SET principal_kind='service',actor_id=NULL WHERE id=?`, id); err != nil {
		t.Fatal("valid service NULL actor rejected", err)
	}
	insert := func(budget, kind, ownerKind string, reserved int64, state string, used, settled any) error {
		_, err := s.DB.Exec(`INSERT INTO platform_repository_budget_reservations(id,budget_id,owner_kind,owner,lease_epoch,kind,reserved,settled_used,state,created_at,settled_at) VALUES(?,?,?,?,0,?,?,?,?,?,?)`, strings.Repeat("d", 64), budget, ownerKind, owner, kind, reserved, used, state, now(), settled)
		return err
	}
	for name, call := range map[string]func() error{
		"orphan_budget":         func() error { return insert(strings.Repeat("e", 64), "request", "admission", 1, "open", nil, nil) },
		"worker_without_run":    func() error { return insert(id, "request", "worker", 1, "open", nil, nil) },
		"request_refund":        func() error { return insert(id, "request", "admission", 1, "settled", 0, now()) },
		"text_refund":           func() error { return insert(id, "text_match", "admission", 4, "settled", 3, now()) },
		"oversized_grant":       func() error { return insert(id, "api_read", "admission", 1048577, "open", nil, nil) },
		"oversized_match":       func() error { return insert(id, "text_match", "admission", 4000001, "open", nil, nil) },
		"unknown_claimed_usage": func() error { return insert(id, "api_read", "admission", 4, "charged_unknown", 2, now()) },
		"settled_without_time":  func() error { return insert(id, "request", "admission", 1, "settled", 1, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	if err = insert(id, "request", "admission", 1, "open", nil, nil); err != nil {
		t.Fatal("valid reservation rejected", err)
	}
	// These schema tests are not runtime authorization or a working ledger.
	var n int
	if err = s.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM platform_repository_budget_reservations`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}
