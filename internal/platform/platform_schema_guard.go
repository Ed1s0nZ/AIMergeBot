package platform

import (
	"database/sql"
	"errors"
	"fmt"
)

var errPlatformSchemaMetadata = errors.New("invalid platform schema metadata; offline repair required")

// Inspect before any migration statement. An absent version table is a legacy
// database, but an empty/duplicated/malformed existing table is not a fresh DB.
func inspectPlatformSchema(tx *sql.Tx) (int64, error) {
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='platform_schema' COLLATE NOCASE)`).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		var future bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE lower(name) IN ('platform_repository_identities','platform_repository_recovery_receipts','platform_legacy_repository_state'))`).Scan(&future); err != nil {
			return 0, err
		}
		if future {
			return 0, errPlatformSchemaMetadata
		}
		return 0, nil
	}
	var kind string
	if err := tx.QueryRow(`SELECT type FROM sqlite_master WHERE name='platform_schema' COLLATE NOCASE`).Scan(&kind); err != nil || kind != "table" {
		return 0, errPlatformSchemaMetadata
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM platform_schema`).Scan(&count); err != nil || count != 1 {
		return 0, errPlatformSchemaMetadata
	}
	var raw any
	var storage string
	if err := tx.QueryRow(`SELECT version,typeof(version) FROM platform_schema`).Scan(&raw, &storage); err != nil {
		return 0, errPlatformSchemaMetadata
	}
	version, ok := raw.(int64)
	if !ok || storage != "integer" {
		return 0, errPlatformSchemaMetadata
	}
	return version, nil
}

func requireCurrentPlatformSchema(tx *sql.Tx) error {
	version, err := inspectPlatformSchema(tx)
	if err != nil {
		return err
	}
	// Zero is accepted only when the table does not exist; inspect returns a
	// separate unsupported result below for an explicitly stored zero.
	if version == 0 {
		var exists bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name='platform_schema' COLLATE NOCASE)`).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return requireStoppedSchemaUpgrade(tx)
		}
	}
	if version == platformSchemaVersion {
		return validateRepositorySchemaTables(tx)
	}
	if version != 1 {
		return fmt.Errorf("unsupported platform schema %d", version)
	}
	return requireStoppedSchemaUpgrade(tx)
}
