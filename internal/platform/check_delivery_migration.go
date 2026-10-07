package platform

import "database/sql"

func migrateCheckDeliveries(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_check_deliveries(run_id INTEGER PRIMARY KEY REFERENCES platform_runs(id),head_sha TEXT NOT NULL,actor INTEGER NOT NULL,automatic INTEGER NOT NULL DEFAULT 0,state TEXT NOT NULL DEFAULT 'pending',blocking INTEGER NOT NULL DEFAULT 0,remote_id INTEGER NOT NULL DEFAULT 0,code TEXT NOT NULL DEFAULT '',lease TEXT NOT NULL DEFAULT '',lease_until TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_check_collector(id INTEGER PRIMARY KEY CHECK(id=1),last_run INTEGER NOT NULL DEFAULT 0)`,
		`INSERT OR IGNORE INTO platform_check_collector(id) VALUES(1)`,
		`CREATE INDEX IF NOT EXISTS platform_check_pending ON platform_check_deliveries(state,run_id)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	rows, err := tx.Query(`PRAGMA table_info(platform_check_deliveries)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var id, notNull, pk int
		var name, kind string
		var def sql.NullString
		if err = rows.Scan(&id, &name, &kind, &notNull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "automatic" {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !found {
		_, err = tx.Exec(`ALTER TABLE platform_check_deliveries ADD COLUMN automatic INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}
