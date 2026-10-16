package platform

import (
	"database/sql"
	"fmt"
	"strings"
)

const platformSchemaVersion = 3

var repositoryBudgetColumns = map[string]string{
	"platform_repository_budgets":             "id,purpose,related_run_id,root_run_id,project_id,principal_kind,actor_id,principal_json,principal_digest,limits_json,limits_digest,state,session_epoch,admission_owner,admission_lease_until,admission_deadline,admission_seed_json,admission_seed_digest,api_charged,git_charged,upload_charged,download_charged,requests_charged,match_cells_charged,created_at,updated_at",
	"platform_run_repository_budgets":         "run_id,budget_id",
	"platform_repository_budget_reservations": "id,budget_id,run_id,owner_kind,owner,lease_epoch,kind,reserved,settled_used,state,created_at,settled_at",
}

func validateRepositoryBudgetSchemaTables(tx *sql.Tx) error {
	for name, columns := range repositoryBudgetColumns {
		var table bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name=? COLLATE NOCASE AND type='table')`, name).Scan(&table); err != nil || !table {
			return errPlatformSchemaMetadata
		}
		// Names are internal constants, never request or database values.
		rows, err := tx.Query("SELECT " + columns + " FROM " + name + " WHERE 0")
		if err != nil {
			return errPlatformSchemaMetadata
		}
		if err := rows.Close(); err != nil {
			return errPlatformSchemaMetadata
		}
	}
	return nil
}

func migrateRepositoryBudgetSchema(tx *sql.Tx) error {
	constraints := []string{}
	for _, pair := range [][2]string{{"api_charged", "api_response_bytes"}, {"git_charged", "git_response_bytes"}, {"upload_charged", "upload_bytes"}, {"download_charged", "download_bytes"}, {"requests_charged", "requests"}, {"match_cells_charged", "text_match_cells"}} {
		column, limit := pair[0], pair[1]
		constraints = append(constraints, fmt.Sprintf(`CHECK(typeof(%s)='integer' AND %s>=0 AND json_extract(limits_json,'$.%s') IS NOT NULL AND json_type(limits_json,'$.%s')='integer' AND %s<=json_extract(limits_json,'$.%s'))`, column, column, limit, limit, column, limit))
	}
	statements := []string{
		`CREATE TABLE platform_repository_budgets(
		 id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=64 AND id NOT GLOB '*[^0-9a-f]*'),
		 purpose TEXT NOT NULL CHECK(purpose IN ('audit','scope_preview','codeowners','publication','poll','diagnostic')),
		 related_run_id INTEGER REFERENCES platform_runs(id) CHECK(related_run_id IS NULL OR (typeof(related_run_id)='integer' AND related_run_id>0)),
		 root_run_id INTEGER UNIQUE REFERENCES platform_runs(id) CHECK(root_run_id IS NULL OR (typeof(root_run_id)='integer' AND root_run_id>0)),
		 project_id INTEGER NOT NULL REFERENCES platform_projects(id) CHECK(typeof(project_id)='integer' AND project_id>0),
		 principal_kind TEXT NOT NULL CHECK(principal_kind IN ('user','service')),actor_id INTEGER REFERENCES platform_users(id),
		 principal_json TEXT NOT NULL CHECK(json_valid(principal_json) AND length(CAST(principal_json AS BLOB))<=8192),
		 principal_digest TEXT NOT NULL CHECK(length(principal_digest)=64 AND principal_digest NOT GLOB '*[^0-9a-f]*'),
		 limits_json TEXT NOT NULL CHECK(json_valid(limits_json) AND length(CAST(limits_json AS BLOB))<=4096),
		 limits_digest TEXT NOT NULL CHECK(length(limits_digest)=64 AND limits_digest NOT GLOB '*[^0-9a-f]*'),
		 state TEXT NOT NULL CHECK(state IN ('admitting','active','closed','abandoned')),
		 session_epoch INTEGER NOT NULL CHECK(typeof(session_epoch)='integer' AND session_epoch>=0),
		 admission_owner TEXT NOT NULL,admission_lease_until TEXT NOT NULL,
		 admission_deadline TEXT NOT NULL CHECK(julianday(admission_deadline) IS NOT NULL),
		 admission_seed_json TEXT CHECK(admission_seed_json IS NULL OR (json_valid(admission_seed_json) AND length(CAST(admission_seed_json AS BLOB))<=65536)),
		 admission_seed_digest TEXT CHECK(admission_seed_digest IS NULL OR (length(admission_seed_digest)=64 AND admission_seed_digest NOT GLOB '*[^0-9a-f]*')),
		 api_charged INTEGER NOT NULL,git_charged INTEGER NOT NULL,upload_charged INTEGER NOT NULL,download_charged INTEGER NOT NULL,requests_charged INTEGER NOT NULL,match_cells_charged INTEGER NOT NULL,
		 created_at TEXT NOT NULL,updated_at TEXT NOT NULL,
		 CHECK((principal_kind='user' AND actor_id IS NOT NULL AND typeof(actor_id)='integer' AND actor_id>0) OR (principal_kind='service' AND actor_id IS NULL)),
		 CHECK((admission_seed_json IS NULL)=(admission_seed_digest IS NULL)),
		 CHECK(download_charged=api_charged+git_charged),
		 CHECK((state='admitting' AND length(admission_owner)=64 AND admission_owner NOT GLOB '*[^0-9a-f]*' AND julianday(admission_lease_until) IS NOT NULL AND julianday(admission_lease_until)<=julianday(admission_deadline)) OR (state!='admitting' AND admission_owner='' AND admission_lease_until='')),
		 CHECK((purpose='audit' AND related_run_id IS NULL AND ((state IN ('admitting','abandoned') AND root_run_id IS NULL) OR (state IN ('active','closed') AND root_run_id IS NOT NULL))) OR (purpose!='audit' AND root_run_id IS NULL AND state!='active')),
		 ` + strings.Join(constraints, ",") + `)`,
		`CREATE TABLE platform_run_repository_budgets(run_id INTEGER PRIMARY KEY NOT NULL REFERENCES platform_runs(id) CHECK(typeof(run_id)='integer' AND run_id>0),budget_id TEXT NOT NULL REFERENCES platform_repository_budgets(id)) WITHOUT ROWID`,
		`CREATE INDEX platform_run_repository_budget_chain ON platform_run_repository_budgets(budget_id,run_id)`,
		`CREATE TABLE platform_repository_budget_reservations(
		 id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=64 AND id NOT GLOB '*[^0-9a-f]*'),budget_id TEXT NOT NULL REFERENCES platform_repository_budgets(id),run_id INTEGER REFERENCES platform_runs(id) CHECK(run_id IS NULL OR (typeof(run_id)='integer' AND run_id>0)),
		 owner_kind TEXT NOT NULL CHECK(owner_kind IN ('admission','worker')),owner TEXT NOT NULL CHECK(length(owner)=64 AND owner NOT GLOB '*[^0-9a-f]*'),
		 lease_epoch INTEGER NOT NULL CHECK(typeof(lease_epoch)='integer' AND lease_epoch>=0),
		 kind TEXT NOT NULL CHECK(kind IN ('request','api_read','git_read','upload','text_match')),
		 reserved INTEGER NOT NULL CHECK(typeof(reserved)='integer' AND reserved>0),settled_used INTEGER CHECK(settled_used IS NULL OR (typeof(settled_used)='integer' AND settled_used>=0 AND settled_used<=reserved)),
		 state TEXT NOT NULL CHECK(state IN ('open','settled','charged_unknown')),created_at TEXT NOT NULL,settled_at TEXT,
		 CHECK((owner_kind='admission' AND run_id IS NULL) OR (owner_kind='worker' AND run_id IS NOT NULL AND lease_epoch>0)),
		 CHECK((kind='request' AND reserved=1 AND (settled_used IS NULL OR settled_used=1)) OR (kind='text_match' AND reserved<=4000000 AND (settled_used IS NULL OR settled_used=reserved)) OR (kind IN ('api_read','git_read','upload') AND reserved<=1048576)),
		 CHECK((state='open' AND settled_used IS NULL AND settled_at IS NULL) OR (state='settled' AND settled_used IS NOT NULL AND settled_at IS NOT NULL) OR (state='charged_unknown' AND settled_used IS NULL AND settled_at IS NOT NULL)))`,
		`CREATE INDEX platform_repository_reservation_owner ON platform_repository_budget_reservations(budget_id,lease_epoch,owner_kind,owner,state,kind)`,
		`CREATE INDEX platform_repository_admission_recovery ON platform_repository_budgets(state,admission_lease_until)`,
		`UPDATE platform_schema SET version=3`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return validateRepositoryBudgetSchemaTables(tx)
}
