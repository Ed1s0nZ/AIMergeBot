package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	legacy "pr_agent/internal"
)

func oldIdentityDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema1.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, statement := range []string{
		`CREATE TABLE platform_schema(version INTEGER NOT NULL);INSERT INTO platform_schema VALUES(1)`,
		`CREATE TABLE platform_users(id INTEGER PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,role TEXT NOT NULL,disabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);INSERT INTO platform_users VALUES(1,'fixture','synthetic','admin',0,'2026-10-01T00:00:00Z')`,
		`CREATE TABLE platform_projects(id INTEGER PRIMARY KEY,name TEXT NOT NULL,enabled INTEGER NOT NULL);INSERT INTO platform_projects VALUES(1,'current-name',1)`,
		`CREATE TABLE platform_project_repositories(project_id INTEGER PRIMARY KEY,revision INTEGER NOT NULL,provider TEXT NOT NULL,api_origin TEXT NOT NULL,remote_id INTEGER NOT NULL,binding_json TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(provider,api_origin,remote_id))`,
		`CREATE TABLE platform_repository_binding_history(project_id INTEGER NOT NULL,revision INTEGER NOT NULL,binding_json TEXT NOT NULL,actor INTEGER NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(project_id,revision))`,
		`CREATE TABLE platform_repository_project_receipts(actor INTEGER NOT NULL,request_id TEXT NOT NULL,digest TEXT NOT NULL,project_id INTEGER NOT NULL,receipt_json TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(actor,request_id))`,
		`CREATE TABLE platform_project_sync(id INTEGER PRIMARY KEY,generation INTEGER NOT NULL DEFAULT 0,dirty INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL);INSERT INTO platform_project_sync VALUES(1,7,0,'','2026-10-01T00:00:00Z')`,
		`CREATE TABLE platform_runs(id INTEGER PRIMARY KEY,project_id INTEGER NOT NULL,mr_iid INTEGER NOT NULL,source_project_id INTEGER NOT NULL,diff_version_id INTEGER NOT NULL DEFAULT 0,base_sha TEXT NOT NULL,head_sha TEXT NOT NULL,title TEXT NOT NULL,url TEXT NOT NULL,status TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result_json TEXT NOT NULL DEFAULT '{"summary":"retained","findings":[],"coverage_notes":[]}',trace_json TEXT NOT NULL DEFAULT '[]',created_at TEXT NOT NULL,started_at TEXT,finished_at TEXT,requested_by INTEGER NOT NULL,policy_version TEXT NOT NULL);INSERT INTO platform_runs(id,project_id,mr_iid,source_project_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version) VALUES(1,1,1,1,'base','head','fixture','','pending','2026-10-01T00:00:00Z',1,'eino-audit-contract-v50')`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	return path, db
}

func seedIdentityBinding(t *testing.T, db *sql.DB, project int, b RepositoryBinding, current bool) {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO platform_repository_binding_history(project_id,revision,binding_json,actor,created_at) VALUES(?,?,?,1,'fixture')`, project, b.Revision, string(raw)); err != nil {
		t.Fatal(err)
	}
	if current {
		if _, err = db.Exec(`INSERT OR REPLACE INTO platform_project_repositories(project_id,revision,provider,api_origin,remote_id,binding_json,updated_at) VALUES(?,?,?,?,?,?,'fixture')`, project, b.Revision, b.Provider, b.APIOrigin, b.RemoteID, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

func seedIdentityReceipt(t *testing.T, db *sql.DB, project int, b RepositoryBinding, tamper bool) {
	t.Helper()
	request := fmt.Sprintf("12345678-1234-4234-8234-%012d", project)
	expected := int64(1)
	input := RepositoryProjectInput{RequestID: request, Name: "initial-name", ExpectedIntegrationRevision: &expected, Provider: b.Provider, APIOrigin: b.APIOrigin, RemoteID: b.RemoteID, FullName: b.FullName, IntegrationID: b.IntegrationID}
	body, _ := json.Marshal(input)
	sum := sha256.Sum256(body)
	r := RepositoryProjectReceipt{Project: Project{ID: project, Name: input.Name, Enabled: true}, Binding: b, IntegrationRevision: 2}
	if tamper {
		r.Project.Name = "tampered-name"
	}
	raw, _ := json.Marshal(r)
	if _, err := db.Exec(`INSERT INTO platform_repository_project_receipts(actor,request_id,digest,project_id,receipt_json,created_at) VALUES(1,?,?,?,?,'fixture')`, request, hex.EncodeToString(sum[:]), project, string(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryIdentityMigrationClassifiesHistoricalEvidence(t *testing.T) {
	path, db := oldIdentityDatabase(t)
	for project := 2; project <= 9; project++ {
		if _, err := db.Exec(`INSERT INTO platform_projects VALUES(?,'current-name',1)`, project); err != nil {
			t.Fatal(err)
		}
		b := RepositoryBinding{Revision: 1, Provider: "github", APIOrigin: "https://api.github.com", RemoteID: int64(100 + project), FullName: fmt.Sprintf("org/repo-%d", project), IntegrationID: 1}
		seedIdentityBinding(t, db, project, b, project != 5)
		if project == 3 || project == 6 {
			seedIdentityReceipt(t, db, project, b, project == 6)
		}
		if project == 2 || project == 3 {
			b.Revision = 2
			b.RemoteID += 100
			b.FullName = fmt.Sprintf("org/rebound-%d", project)
			seedIdentityBinding(t, db, project, b, true)
		}
	}
	for _, q := range []string{
		`UPDATE platform_project_repositories SET binding_json='{' WHERE project_id=4`,
		`UPDATE platform_project_repositories SET revision='invalid' WHERE project_id=7`,
		`UPDATE platform_repository_binding_history SET binding_json='{}' WHERE project_id=9`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE platform_project_repositories SET binding_json=? WHERE project_id=8`, strings.Repeat(" ", 4097)); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for project := 1; project <= 9; project++ {
		r, err := readRepositoryIdentity(context.Background(), s.DB, project)
		wantKind, wantHealth := "internal", "repair_required"
		if project == 1 {
			wantKind, wantHealth = "legacy", "valid"
		}
		if project == 2 || project == 3 {
			wantHealth = "valid"
		}
		if err != nil || r.Kind != wantKind || r.Health != wantHealth || r.Revision != 1 {
			t.Fatal(project, r, err)
		}
	}
	var version int
	var status, result string
	if err = s.DB.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=1`).Scan(&status, &result); err != nil || status != "failed" || !strings.Contains(result, "retained") {
		t.Fatal(status, result, err)
	}
	var children int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs WHERE retry_parent_id!=0`).Scan(&children); err != nil || children != 0 {
		t.Fatal(children, err)
	}
	var generation int
	var pending bool
	if err = s.DB.QueryRow(`SELECT generation,dirty FROM platform_project_sync WHERE id=1`).Scan(&generation, &pending); err != nil || generation != 8 || !pending {
		t.Fatal(generation, pending, err)
	}
}

func TestRepositoryIdentityMigrationRollbackAndLiveWorker(t *testing.T) {
	for _, mode := range []string{"active", "bad_lease", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			path, db := oldIdentityDatabase(t)
			if mode == "rollback" {
				if _, err := db.Exec(`CREATE TRIGGER stop_upgrade BEFORE UPDATE ON platform_schema BEGIN SELECT RAISE(ABORT,'synthetic-upgrade-stop');END`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`CREATE TABLE platform_worker_instance(id INTEGER PRIMARY KEY,owner TEXT NOT NULL,lease_until TEXT NOT NULL)`); err != nil {
					t.Fatal(err)
				}
				lease := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
				if mode == "bad_lease" {
					lease = "invalid"
				}
				if _, err := db.Exec(`INSERT INTO platform_worker_instance VALUES(1,'old-worker',?)`, lease); err != nil {
					t.Fatal(err)
				}
			}
			before := schemaManifest(t, db)
			s, err := OpenStore(path)
			if s != nil {
				s.Close()
				t.Fatal("invalid upgrade opened")
			}
			if err == nil {
				t.Fatal("expected upgrade failure")
			}
			if mode != "rollback" && !errors.Is(err, ErrWorkerInstanceActive) {
				t.Fatal(err)
			}
			if mode == "rollback" && !strings.Contains(err.Error(), "synthetic-upgrade-stop") {
				t.Fatal(err)
			}
			if after := schemaManifest(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("failed upgrade changed schema")
			}
			var version, generation int
			var status string
			var pending bool
			if err = db.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 1 {
				t.Fatal(version, err)
			}
			if err = db.QueryRow(`SELECT generation,dirty FROM platform_project_sync`).Scan(&generation, &pending); err != nil || generation != 7 || pending {
				t.Fatal(generation, pending, err)
			}
			if err = db.QueryRow(`SELECT status FROM platform_runs`).Scan(&status); err != nil || status != "pending" {
				t.Fatal(status, err)
			}
		})
	}
}

func TestRepositoryIdentityMigrationRejectsExistingNamespaceCollision(t *testing.T) {
	path, db := oldIdentityDatabase(t)
	b := RepositoryBinding{Revision: 1, Provider: "github", APIOrigin: "https://api.github.com", RemoteID: 900, FullName: "org/native", IntegrationID: 1}
	seedIdentityBinding(t, db, 1, b, true)
	seedIdentityReceipt(t, db, 1, b, false)
	before := schemaManifest(t, db)
	s, err := OpenStore(path)
	if s != nil {
		s.Close()
		t.Fatal("ambiguous history reassigned")
	}
	if !errors.Is(err, errRepositoryHistoryCollision) {
		t.Fatal(err)
	}
	if after := schemaManifest(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected collision changed schema")
	}
	var version int
	var status string
	if err = db.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 1 {
		t.Fatal(version, err)
	}
	if err = db.QueryRow(`SELECT status FROM platform_runs WHERE id=1`).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
}

func TestRepositoryIdentityCannotFallBackAfterBindingLoss(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	b, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := readRepositoryIdentity(ctx, s.DB, 1)
	if err != nil || r.Kind != "internal" || r.Revision != 2 {
		t.Fatal(r, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_project_repositories WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	if err = requireLegacyRepositoryProject(ctx, s.DB, 1); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("identity reverted", err)
	}
	if err = s.SaveLegacyProject(ctx, Project{ID: 1, Name: "wrong", Enabled: true}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, b); !errors.Is(err, ErrConflict) {
		t.Fatal("missing binding recreated", err)
	}
	projects, _, _, err := s.projectConfiguration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.ID == 1 && !p.InternalProject {
			t.Fatal("export guessed legacy")
		}
	}
	if _, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, Name: "wrong", InternalProject: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, Name: "wrong"}}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	reopened := reopenPublicationGuardStore(t, s)
	if err = requireLegacyRepositoryProject(ctx, reopened.DB, 1); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("restart guessed legacy", err)
	}
	if _, err = reopened.ProjectsForUser(ctx, admin); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("UI guessed legacy", err)
	}
}

func TestRepositoryIdentityMissingStateIsNeverReinitialized(t *testing.T) {
	for _, target := range []string{"identity", "key_row", "state_table", "receipt_table", "version_table"} {
		t.Run(target, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			var seq int
			var name, path string
			if err := s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
				t.Fatal(err)
			}
			queries := map[string]string{"identity": `DELETE FROM platform_repository_identities WHERE project_id=1`, "key_row": `DELETE FROM platform_legacy_repository_state`, "state_table": `DROP TABLE platform_legacy_repository_state`, "receipt_table": `DROP TABLE platform_repository_recovery_receipts`, "version_table": `DROP TABLE platform_schema`}
			if _, err := s.DB.Exec(queries[target]); err != nil {
				t.Fatal(err)
			}
			before := schemaManifest(t, s.DB)
			var beforeGeneration int64
			var beforeKey, beforeFingerprint []byte
			beforeStateErr := s.DB.QueryRow(`SELECT generation,hmac_key,fingerprint FROM platform_legacy_repository_state WHERE id=1`).Scan(&beforeGeneration, &beforeKey, &beforeFingerprint)
			var beforeCount int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_identities`).Scan(&beforeCount); err != nil {
				t.Fatal(err)
			}
			if reopened, err := OpenStore(path); err == nil {
				reopened.Close()
				t.Fatal("damaged current state recreated")
			}
			if after := schemaManifest(t, s.DB); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected open changed metadata")
			}
			var afterGeneration int64
			var afterKey, afterFingerprint []byte
			afterStateErr := s.DB.QueryRow(`SELECT generation,hmac_key,fingerprint FROM platform_legacy_repository_state WHERE id=1`).Scan(&afterGeneration, &afterKey, &afterFingerprint)
			if (beforeStateErr == nil) != (afterStateErr == nil) || beforeGeneration != afterGeneration || !reflect.DeepEqual(beforeKey, afterKey) || !reflect.DeepEqual(beforeFingerprint, afterFingerprint) {
				t.Fatal("rejected open changed private state")
			}
			var afterCount int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_identities`).Scan(&afterCount); err != nil || afterCount != beforeCount {
				t.Fatal("rejected open backfilled identity", err)
			}
		})
	}
}

func TestRepositoryIdentityNewNativeAndRevisionOverflow(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := repositoryProfile(t, s, admin.ID, "github")
	receipt, err := s.CreateRepositoryProject(ctx, admin.ID, repositoryProjectInput(v, "99999999-9999-4999-8999-999999999999"))
	if err != nil {
		t.Fatal(err)
	}
	id := receipt.Project.ID
	r, err := readRepositoryIdentity(ctx, s.DB, id)
	if err != nil || r.Kind != "internal" || r.Revision != 1 || r.Health != "valid" {
		t.Fatal(r, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_repository_identities SET revision=9223372036854775807 WHERE project_id=?`, id); err != nil {
		t.Fatal(err)
	}
	beforeSync, err := s.ProjectSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var beforeEvents int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	changed := receipt.Binding
	changed.RemoteID++
	if _, err = s.SaveRepositoryBinding(ctx, id, admin.ID, receipt.Binding.Revision, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("overflow accepted", err)
	}
	stored, err := s.RepositoryBinding(ctx, id, admin.ID)
	if err != nil || stored != receipt.Binding {
		t.Fatal("partial binding write", err)
	}
	afterSync, err := s.ProjectSyncStatus(ctx)
	if err != nil || afterSync != beforeSync {
		t.Fatal("partial sync write", err)
	}
	var afterEvents, history int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events`).Scan(&afterEvents); err != nil || afterEvents != beforeEvents {
		t.Fatal("partial event write", err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history WHERE project_id=?`, id).Scan(&history); err != nil || history != 1 {
		t.Fatal("partial history write", err)
	}
}
