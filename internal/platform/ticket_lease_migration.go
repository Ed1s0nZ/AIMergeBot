package platform

import "database/sql"

func migrateTicketLease(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(platform_ticket_links)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
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
	for _, column := range []string{"lease", "lease_until", "error_code", "endpoint_origin"} {
		if !columns[column] {
			if _, err := tx.Exec(`ALTER TABLE platform_ticket_links ADD COLUMN ` + column + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS platform_ticket_ready ON platform_ticket_links(state,id)`)
	return err
}
