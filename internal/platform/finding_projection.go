package platform

import (
	"database/sql"
	"fmt"
)

// The result remains authoritative. AFTER triggers maintain a payload-free
// index in the same transaction as every result writer, including imports.
func migrateFindingProjection(tx *sql.Tx) error {
	const extract = `INSERT INTO platform_finding_index(run_id,ordinal,finding_id,severity,kind)
SELECT NEW.id,CAST(f.key AS INTEGER),COALESCE(json_extract(f.value,'$.id'),''),COALESCE(json_extract(f.value,'$.severity'),''),COALESCE(json_extract(f.value,'$.type'),'')
FROM json_each(NEW.result_json,'$.findings') f;`
	queries := []string{
		`CREATE TABLE IF NOT EXISTS platform_finding_index(run_id INTEGER NOT NULL,ordinal INTEGER NOT NULL,finding_id TEXT NOT NULL,severity TEXT NOT NULL,kind TEXT NOT NULL,PRIMARY KEY(run_id,ordinal))`,
		`CREATE INDEX IF NOT EXISTS platform_finding_severity ON platform_finding_index(run_id,severity)`,
		`CREATE INDEX IF NOT EXISTS platform_finding_kind ON platform_finding_index(run_id,kind)`,
		`CREATE INDEX IF NOT EXISTS platform_finding_identity ON platform_finding_index(run_id,finding_id)`,
		`CREATE TABLE IF NOT EXISTS platform_projection_schema(name TEXT PRIMARY KEY,version INTEGER NOT NULL)`,
		`CREATE TRIGGER IF NOT EXISTS platform_finding_insert AFTER INSERT ON platform_runs BEGIN ` + extract + ` END`,
		`CREATE TRIGGER IF NOT EXISTS platform_finding_update AFTER UPDATE OF result_json ON platform_runs BEGIN DELETE FROM platform_finding_index WHERE run_id=NEW.id; ` + extract + ` END`,
		`CREATE TRIGGER IF NOT EXISTS platform_finding_delete AFTER DELETE ON platform_runs BEGIN DELETE FROM platform_finding_index WHERE run_id=OLD.id; END`,
	}
	for _, q := range queries {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	var version int
	err := tx.QueryRow(`SELECT version FROM platform_projection_schema WHERE name='findings'`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err = tx.Exec(`INSERT INTO platform_finding_index(run_id,ordinal,finding_id,severity,kind)
SELECT r.id,CAST(f.key AS INTEGER),COALESCE(json_extract(f.value,'$.id'),''),COALESCE(json_extract(f.value,'$.severity'),''),COALESCE(json_extract(f.value,'$.type'),'') FROM platform_runs r,json_each(r.result_json,'$.findings') f`); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO platform_projection_schema(name,version) VALUES('findings',1)`)
		version = 1
	}
	if err == nil && version != 1 {
		return fmt.Errorf("unsupported finding projection version %d", version)
	}
	return err
}
