package platform

import "database/sql"

func migrateAuditIdentity(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(platform_runs)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range []struct{ name, definition string }{{"policy_digest", "TEXT NOT NULL DEFAULT ''"}, {"audit_policy_json", "TEXT NOT NULL DEFAULT 'null'"}, {"retry_parent_id", "INTEGER NOT NULL DEFAULT 0"}, {"retry_attempt", "INTEGER NOT NULL DEFAULT 0"}, {"retry_at", "TEXT NOT NULL DEFAULT ''"}} {
		if !columns[c.name] {
			if _, err = tx.Exec(`ALTER TABLE platform_runs ADD COLUMN ` + c.name + ` ` + c.definition); err != nil {
				return err
			}
		}
	}
	for _, q := range []string{`CREATE INDEX IF NOT EXISTS platform_retry_parent ON platform_runs(retry_parent_id)`, `DROP INDEX IF EXISTS platform_active_run`, `DROP INDEX IF EXISTS platform_snapshot_run`, `CREATE UNIQUE INDEX platform_active_run ON platform_runs(project_id,mr_iid,base_sha,head_sha,policy_version,policy_digest) WHERE status IN ('pending','running')`, `CREATE INDEX platform_snapshot_run ON platform_runs(project_id,mr_iid,base_sha,head_sha,policy_version,policy_digest,id)`} {
		if _, err = tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
