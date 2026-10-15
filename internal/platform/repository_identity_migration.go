package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
)

const platformSchemaVersion = 2

func validateRepositorySchemaTables(tx *sql.Tx) error {
	for _, name := range []string{"platform_repository_identities", "platform_repository_recovery_receipts", "platform_legacy_repository_state"} {
		var found bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name=? COLLATE NOCASE AND type='table')`, name).Scan(&found); err != nil || !found {
			return errPlatformSchemaMetadata
		}
	}
	var count, generation int64
	var key, fingerprint []byte
	var generationType, keyType, fingerprintType string
	if err := tx.QueryRow(`SELECT COUNT(*) FROM platform_legacy_repository_state`).Scan(&count); err != nil || count != 1 {
		return errPlatformSchemaMetadata
	}
	if err := tx.QueryRow(`SELECT generation,hmac_key,fingerprint,typeof(generation),typeof(hmac_key),typeof(fingerprint) FROM platform_legacy_repository_state WHERE id=1`).Scan(&generation, &key, &fingerprint, &generationType, &keyType, &fingerprintType); err != nil || generationType != "integer" || keyType != "blob" || generation < 0 || len(key) != 32 || (generation == 0 && (fingerprint != nil || fingerprintType != "null")) || (generation > 0 && (len(fingerprint) != 32 || fingerprintType != "blob")) {
		return errPlatformSchemaMetadata
	}
	var missing bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM platform_projects p WHERE NOT EXISTS(SELECT 1 FROM platform_repository_identities r WHERE r.project_id=p.id))`).Scan(&missing); err != nil || missing {
		return errPlatformSchemaMetadata
	}
	return nil
}

func requireStoppedSchemaUpgrade(tx *sql.Tx) error {
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='platform_worker_instance' COLLATE NOCASE)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	var active bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM platform_worker_instance WHERE owner!='' AND (julianday(lease_until) IS NULL OR julianday(lease_until)>julianday(?)))`, now()).Scan(&active); err != nil {
		return err
	}
	if active {
		return ErrWorkerInstanceActive
	}
	return nil
}

func migrateRepositoryIdentities(tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE platform_repository_identities(project_id INTEGER PRIMARY KEY REFERENCES platform_projects(id),kind TEXT NOT NULL CHECK(kind IN ('legacy','internal')),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>0),health TEXT NOT NULL CHECK(health IN ('valid','repair_required')),created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE platform_repository_recovery_receipts(actor INTEGER NOT NULL REFERENCES platform_users(id),request_id TEXT NOT NULL,project_id INTEGER NOT NULL REFERENCES platform_projects(id),digest TEXT NOT NULL CHECK(length(digest)=64),identity_revision INTEGER NOT NULL CHECK(typeof(identity_revision)='integer' AND identity_revision>0),receipt_json TEXT NOT NULL CHECK(length(CAST(receipt_json AS BLOB))<=16384),created_at TEXT NOT NULL,PRIMARY KEY(actor,request_id))`,
		`CREATE TABLE platform_legacy_repository_state(id INTEGER PRIMARY KEY CHECK(id=1),generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>=0),hmac_key BLOB NOT NULL CHECK(typeof(hmac_key)='blob' AND length(hmac_key)=32),fingerprint BLOB CHECK(fingerprint IS NULL OR (typeof(fingerprint)='blob' AND length(fingerprint)=32)),updated_at TEXT NOT NULL,CHECK((generation=0 AND fingerprint IS NULL) OR (generation>0 AND fingerprint IS NOT NULL)))`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO platform_legacy_repository_state(id,generation,hmac_key,updated_at) VALUES(1,0,?,?)`, key, now()); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT id FROM platform_projects ORDER BY id`)
	if err != nil {
		return err
	}
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id <= 0 {
			return errPlatformSchemaMetadata
		}
		kind, health, err := classifyRepositoryIdentity(context.Background(), tx, id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO platform_repository_identities(project_id,kind,revision,health,created_at,updated_at) VALUES(?,?,1,?,?,?)`, id, kind, health, now(), now()); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		if err = markProjectSync(tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE platform_runs SET status='failed',error='repository schema upgraded; resubmit audit',finished_at=? WHERE status IN ('pending','running')`, now()); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE platform_schema SET version=?`, platformSchemaVersion); err != nil {
		return err
	}
	return validateRepositorySchemaTables(tx)
}
