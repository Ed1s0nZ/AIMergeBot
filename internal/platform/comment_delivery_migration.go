package platform

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
)

func migrateCommentDelivery(tx *sql.Tx) error {
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	queries := []string{
		`CREATE TABLE IF NOT EXISTS platform_comment_installation(id INTEGER PRIMARY KEY CHECK(id=1),namespace TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_comment_delivery(run_id INTEGER PRIMARY KEY REFERENCES platform_runs(id),desired_generation INTEGER NOT NULL DEFAULT 1,sent_generation INTEGER NOT NULL DEFAULT 0,state TEXT NOT NULL DEFAULT 'pending',discussion_id TEXT NOT NULL DEFAULT '',note_id INTEGER NOT NULL DEFAULT 0,author_id INTEGER NOT NULL DEFAULT 0,body_hash TEXT NOT NULL DEFAULT '',attempted_hash TEXT NOT NULL DEFAULT '',claimed_generation INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',claim_owner TEXT NOT NULL DEFAULT '',claim_until TEXT NOT NULL DEFAULT '',retry_at TEXT NOT NULL DEFAULT '',attempts INTEGER NOT NULL DEFAULT 0,review_actor INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS platform_comment_pending ON platform_comment_delivery(state,retry_at,run_id)`,
		`CREATE TRIGGER IF NOT EXISTS platform_comment_result AFTER UPDATE OF status ON platform_runs WHEN NEW.status='succeeded' AND OLD.status<>'succeeded' BEGIN INSERT OR IGNORE INTO platform_comment_delivery(run_id,updated_at) VALUES(NEW.id,NEW.finished_at); END`,
	}
	for _, q := range queries {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO platform_comment_installation(id,namespace) VALUES(1,?)`, hex.EncodeToString(seed)); err != nil {
		return err
	}
	// A legacy delivery has no stable identity or acknowledgement IDs. Never
	// reinterpret it as permission to create a duplicate external discussion.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO platform_comment_delivery(run_id,state,last_error,updated_at) SELECT run_id,'blocked','Legacy delivery identity unavailable',updated_at FROM platform_comments`); err != nil {
		return err
	}
	for _, event := range []string{"INSERT", "UPDATE"} {
		q := `CREATE TRIGGER IF NOT EXISTS platform_comment_review_` + event + ` AFTER ` + event + ` ON platform_reviews BEGIN
INSERT INTO platform_comment_delivery(run_id,desired_generation,review_actor,updated_at)
SELECT NEW.run_id,1,NEW.actor,NEW.updated_at FROM platform_runs WHERE id=NEW.run_id AND status='succeeded'
ON CONFLICT(run_id) DO UPDATE SET desired_generation=desired_generation+1,review_actor=NEW.actor,updated_at=NEW.updated_at,attempts=CASE WHEN state IN ('sent','pending') THEN 0 ELSE attempts END,retry_at=CASE WHEN state IN ('sent','pending') THEN '' ELSE retry_at END,state=CASE WHEN state IN ('sent','pending') THEN 'pending' ELSE state END;
END`
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
