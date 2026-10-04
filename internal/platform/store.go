package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	DB            *sql.DB
	projectSyncMu sync.Mutex
	quotaSettings atomic.Pointer[SettingsService]
}

func OpenStore(path string) (*Store, error) {
	dsn := "file:" + url.PathEscape(path) + "?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS platform_project_sync (id INTEGER PRIMARY KEY CHECK(id=1),generation INTEGER NOT NULL DEFAULT 0,dirty INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO platform_project_sync(id,updated_at) VALUES(1,'')`,
		`CREATE TABLE IF NOT EXISTS platform_legacy_imports (legacy_id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL REFERENCES platform_runs(id))`,
		`CREATE TABLE IF NOT EXISTS platform_poll_seen (project_id INTEGER NOT NULL,mr_iid INTEGER NOT NULL,head_sha TEXT NOT NULL,PRIMARY KEY(project_id,mr_iid))`,
		`CREATE TABLE IF NOT EXISTS platform_comments (run_id INTEGER PRIMARY KEY,status TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_schema (version INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_users (id INTEGER PRIMARY KEY,username TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,role TEXT NOT NULL CHECK(role IN ('admin','member')),disabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_sessions (token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES platform_users(id),expires_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_projects (id INTEGER PRIMARY KEY,name TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS platform_runs (id INTEGER PRIMARY KEY,project_id INTEGER NOT NULL,mr_iid INTEGER NOT NULL,source_project_id INTEGER NOT NULL,diff_version_id INTEGER NOT NULL DEFAULT 0,base_sha TEXT NOT NULL,head_sha TEXT NOT NULL,title TEXT NOT NULL,url TEXT NOT NULL,status TEXT NOT NULL,error TEXT NOT NULL DEFAULT '',result_json TEXT NOT NULL DEFAULT '{"findings":[],"summary":"","coverage_notes":[]}',trace_json TEXT NOT NULL DEFAULT '[]',created_at TEXT NOT NULL,started_at TEXT,finished_at TEXT,requested_by INTEGER NOT NULL,policy_version TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS platform_active_run ON platform_runs(project_id,mr_iid,head_sha,policy_version) WHERE status IN ('pending','running')`,
		`CREATE INDEX IF NOT EXISTS platform_snapshot_run ON platform_runs(project_id,mr_iid,head_sha,policy_version,id)`,
		`CREATE TABLE IF NOT EXISTS platform_reviews (run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,status TEXT NOT NULL,reason TEXT NOT NULL,actor INTEGER NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(run_id,finding_id))`,
		`CREATE TABLE IF NOT EXISTS platform_events (id INTEGER PRIMARY KEY,actor INTEGER NOT NULL,action TEXT NOT NULL,target TEXT NOT NULL,created_at TEXT NOT NULL)`,
		`INSERT INTO platform_schema(version) SELECT 1 WHERE NOT EXISTS(SELECT 1 FROM platform_schema)`,
	}
	for _, q := range statements {
		if _, err = tx.Exec(q); err != nil {
			return fmt.Errorf("platform migration: %w", err)
		}
	}
	columns, err := tx.Query(`PRAGMA table_info(platform_runs)`)
	if err != nil {
		return err
	}
	hasVersion := false
	for columns.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err = columns.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			columns.Close()
			return err
		}
		if name == "diff_version_id" {
			hasVersion = true
		}
	}
	err = columns.Err()
	columns.Close()
	if err != nil {
		return err
	}
	if !hasVersion {
		if _, err = tx.Exec(`ALTER TABLE platform_runs ADD COLUMN diff_version_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if err = migrateAuditIdentity(tx); err != nil {
		return err
	}
	if err = migrateWorkerLease(tx); err != nil {
		return err
	}
	if err = migrateProjectAccess(tx); err != nil {
		return err
	}
	for _, query := range []string{
		`CREATE INDEX IF NOT EXISTS platform_quota_project_running ON platform_runs(status,project_id)`,
		`CREATE INDEX IF NOT EXISTS platform_quota_user_running ON platform_runs(status,requested_by)`,
		`CREATE INDEX IF NOT EXISTS platform_quota_started ON platform_runs(julianday(started_at))`,
		`CREATE INDEX IF NOT EXISTS platform_quota_project_started ON platform_runs(project_id,julianday(started_at))`,
		`CREATE INDEX IF NOT EXISTS platform_quota_user_started ON platform_runs(requested_by,julianday(started_at))`,
	} {
		if _, err = tx.Exec(query); err != nil {
			return err
		}
	}
	var version int
	if err = tx.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported platform schema %d", version)
	}
	return tx.Commit()
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) Event(ctx context.Context, actor int64, action, target string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,?,?,?)`, actor, action, target, now())
	return err
}

func (s *Store) Projects(ctx context.Context) ([]Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,enabled FROM platform_projects ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		var p Project
		if err = rows.Scan(&p.ID, &p.Name, &p.Enabled); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (s *Store) SaveProject(ctx context.Context, p Project) error {
	if p.ID <= 0 || len(p.Name) == 0 || len(p.Name) > 200 {
		return fmt.Errorf("invalid project")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_projects(id,name,enabled) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,enabled=excluded.enabled`, p.ID, p.Name, p.Enabled)
	if err != nil {
		return err
	}
	if err = markProjectSync(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Run(ctx context.Context, id int64) (Run, error) { return s.readRun(ctx, id, true) }

// RunSummary avoids loading potentially large tool traces for list responses.
func (s *Store) RunSummary(ctx context.Context, id int64) (Run, error) {
	return s.readRun(ctx, id, false)
}
func (s *Store) readRun(ctx context.Context, id int64, includeTrace bool) (Run, error) {
	var r Run
	var result, trace, created, policy, retryInfo string
	var started, finished sql.NullString
	traceColumn := "trace_json"
	if !includeTrace {
		traceColumn = "'[]'"
	}
	err := s.DB.QueryRowContext(ctx, `SELECT id,project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,error,result_json,`+traceColumn+`,created_at,started_at,finished_at,requested_by,policy_version,audit_policy_json,retry_parent_id,retry_attempt,retry_at,retry_info_json,worker_owner,worker_lease_until,COALESCE((SELECT child.id FROM platform_runs child WHERE child.retry_parent_id=platform_runs.id ORDER BY child.id DESC LIMIT 1),0) FROM platform_runs WHERE id=?`, id).Scan(&r.ID, &r.ProjectID, &r.MRIID, &r.SourceProjectID, &r.DiffVersionID, &r.BaseSHA, &r.HeadSHA, &r.Title, &r.URL, &r.Status, &r.Error, &result, &trace, &created, &started, &finished, &r.RequestedBy, &r.PolicyVersion, &policy, &r.RetryParentID, &r.RetryAttempt, &r.RetryAt, &retryInfo, &r.WorkerOwner, &r.WorkerLeaseUntil, &r.RetryChildID)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(retryInfo), &r.RetryInfo); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(policy), &r.AuditPolicy); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(result), &r.Result); err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(trace), &r.Trace); err != nil {
		return r, err
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return r, err
	}
	if started.Valid {
		t, e := time.Parse(time.RFC3339Nano, started.String)
		if e != nil {
			return r, e
		}
		r.StartedAt = &t
	}
	if finished.Valid {
		t, e := time.Parse(time.RFC3339Nano, finished.String)
		if e != nil {
			return r, e
		}
		r.FinishedAt = &t
	}
	return r, nil
}
