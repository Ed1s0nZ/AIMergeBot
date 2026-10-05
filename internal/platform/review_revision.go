package platform

import (
	"database/sql"
	"errors"
)

var ErrReviewConflict = errors.New("review changed; reload the latest decision")
var ErrReviewRevision = errors.New("expected_revision is required and must be nonnegative")

func migrateReviewRevision(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(platform_reviews)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, required, primary int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &required, &defaultValue, &primary); err != nil {
			rows.Close()
			return err
		}
		found = found || name == "revision"
	}
	err = rows.Err()
	rows.Close()
	if err != nil || found {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE platform_reviews ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`)
	return err
}
