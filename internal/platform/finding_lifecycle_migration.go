package platform

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func migrateFindingLifecycle(tx *sql.Tx) error {
	const extract = `INSERT OR REPLACE INTO platform_finding_occurrences(run_id,finding_id,fingerprint)
SELECT NEW.id,json_extract(f.value,'$.id'),json_extract(f.value,'$.fingerprint') FROM json_each(NEW.result_json,'$.findings') f
WHERE length(json_extract(f.value,'$.fingerprint'))=64 AND json_extract(f.value,'$.fingerprint') NOT GLOB '*[^0-9a-f]*' AND length(json_extract(f.value,'$.id'))>0;`
	queries := []string{
		`CREATE TABLE IF NOT EXISTS platform_finding_occurrences(run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,fingerprint TEXT NOT NULL,PRIMARY KEY(run_id,finding_id))`,
		`CREATE INDEX IF NOT EXISTS platform_finding_fingerprint ON platform_finding_occurrences(fingerprint,run_id)`,
		`CREATE TABLE IF NOT EXISTS platform_review_history(id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL REFERENCES platform_runs(id),finding_id TEXT NOT NULL,status TEXT NOT NULL,reason TEXT NOT NULL,actor INTEGER NOT NULL,created_at TEXT NOT NULL,imported INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS platform_review_history_finding ON platform_review_history(run_id,finding_id,id)`,
		`CREATE TRIGGER IF NOT EXISTS platform_occurrence_insert AFTER INSERT ON platform_runs BEGIN ` + extract + ` END`,
		`CREATE TRIGGER IF NOT EXISTS platform_occurrence_update AFTER UPDATE OF result_json ON platform_runs BEGIN DELETE FROM platform_finding_occurrences WHERE run_id=NEW.id; ` + extract + ` END`,
		`CREATE TRIGGER IF NOT EXISTS platform_occurrence_delete AFTER DELETE ON platform_runs BEGIN DELETE FROM platform_finding_occurrences WHERE run_id=OLD.id; END`,
		`CREATE TRIGGER IF NOT EXISTS platform_review_history_insert AFTER INSERT ON platform_reviews BEGIN INSERT INTO platform_review_history(run_id,finding_id,status,reason,actor,created_at) VALUES(NEW.run_id,NEW.finding_id,NEW.status,NEW.reason,NEW.actor,NEW.updated_at); END`,
		`CREATE TRIGGER IF NOT EXISTS platform_review_history_update AFTER UPDATE ON platform_reviews BEGIN INSERT INTO platform_review_history(run_id,finding_id,status,reason,actor,created_at) VALUES(NEW.run_id,NEW.finding_id,NEW.status,NEW.reason,NEW.actor,NEW.updated_at); END`,
	}
	for _, q := range queries {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	var version int
	err := tx.QueryRow(`SELECT version FROM platform_projection_schema WHERE name='finding_lifecycle'`).Scan(&version)
	if err == nil {
		if version != 1 {
			return fmt.Errorf("unsupported finding lifecycle version %d", version)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	// Bounded keyset migration; never rewrite historical result or trace bytes.
	last := int64(0)
	for {
		rows, err := tx.Query(`SELECT id,project_id,mr_iid,source_project_id,base_sha,head_sha,result_json FROM platform_runs WHERE id>? AND policy_version<>'legacy' ORDER BY id LIMIT 200`, last)
		if err != nil {
			return err
		}
		type entry struct {
			id  int64
			s   Snapshot
			raw string
		}
		entries := []entry{}
		for rows.Next() {
			var e entry
			if err = rows.Scan(&e.id, &e.s.ProjectID, &e.s.MRIID, &e.s.SourceProjectID, &e.s.BaseSHA, &e.s.HeadSHA, &e.raw); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			break
		}
		for _, e := range entries {
			var result AuditResult
			if err = json.Unmarshal([]byte(e.raw), &result); err != nil {
				return fmt.Errorf("lifecycle backfill run %d: %w", e.id, err)
			}
			counts := map[string]int{}
			for _, f := range result.Findings {
				counts[findingFingerprint(e.s, f)]++
			}
			for _, f := range result.Findings {
				fingerprint := findingFingerprint(e.s, f)
				if fingerprint == "" || f.ID == "" || counts[fingerprint] > 1 {
					continue
				}
				if _, err = tx.Exec(`INSERT OR REPLACE INTO platform_finding_occurrences(run_id,finding_id,fingerprint) VALUES(?,?,?)`, e.id, f.ID, fingerprint); err != nil {
					return err
				}
			}
			last = e.id
		}
	}
	if _, err = tx.Exec(`INSERT INTO platform_review_history(run_id,finding_id,status,reason,actor,created_at,imported) SELECT run_id,finding_id,status,reason,actor,updated_at,1 FROM platform_reviews`); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO platform_projection_schema(name,version) VALUES('finding_lifecycle',1)`)
	return err
}
