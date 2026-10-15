package platform

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func schemaManifest(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT type||'|'||name||'|'||COALESCE(sql,'') FROM sqlite_master ORDER BY type,name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var entry string
		if err = rows.Scan(&entry); err != nil {
			t.Fatal(err)
		}
		out = append(out, entry)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPlatformSchemaRejectsInvalidMetadataBeforeMigration(t *testing.T) {
	cases := map[string]string{
		"empty":               `CREATE TABLE platform_schema(version INTEGER)`,
		"duplicate_current":   `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(1),(1)`,
		"current_then_future": `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(1),(2)`,
		"future":              `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(3)`,
		"uppercase_future":    `CREATE TABLE PLATFORM_SCHEMA(version INTEGER);INSERT INTO PLATFORM_SCHEMA VALUES(3)`,
		"incomplete_current":  `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(2)`,
		"zero":                `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(0)`,
		"negative":            `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(-1)`,
		"text":                `CREATE TABLE platform_schema(version TEXT);INSERT INTO platform_schema VALUES('1')`,
		"float":               `CREATE TABLE platform_schema(version REAL);INSERT INTO platform_schema VALUES(1.0)`,
		"null":                `CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(NULL)`,
		"missing_column":      `CREATE TABLE platform_schema(wrong INTEGER);INSERT INTO platform_schema VALUES(1)`,
		"view":                `CREATE VIEW platform_schema AS SELECT 1 AS version`,
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.db")
			db, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.Exec(setup); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`CREATE TABLE sentinel(value TEXT);INSERT INTO sentinel VALUES('preserved')`); err != nil {
				t.Fatal(err)
			}
			// A synthetic migration write would fail loudly if version inspection
			// were postponed until after the first migration statement.
			if _, err = db.Exec(`CREATE TABLE platform_project_sync(id INTEGER PRIMARY KEY,generation INTEGER NOT NULL DEFAULT 0,dirty INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',updated_at TEXT NOT NULL);CREATE TRIGGER migration_write_trap BEFORE INSERT ON platform_project_sync BEGIN SELECT RAISE(ABORT,'migration-write-before-version-check');END`); err != nil {
				t.Fatal(err)
			}
			before := schemaManifest(t, db)
			s, err := OpenStore(path)
			if s != nil {
				s.Close()
				t.Fatal("accepted incompatible database")
			}
			if err == nil || strings.Contains(err.Error(), "migration-write-before-version-check") || (!strings.Contains(err.Error(), "platform schema") && !strings.Contains(err.Error(), "schema metadata")) {
				t.Fatal("migration ran before preflight", err)
			}
			if after := schemaManifest(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected open changed schema")
			}
			var value string
			if err = db.QueryRow(`SELECT value FROM sentinel`).Scan(&value); err != nil || value != "preserved" {
				t.Fatal(value, err)
			}
		})
	}
}

func TestPlatformSchemaCurrentAndUnversionedRemainCompatible(t *testing.T) {
	for _, versioned := range []bool{false, true} {
		t.Run(map[bool]string{false: "unversioned", true: "current"}[versioned], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.db")
			db, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			if versioned {
				if _, err = db.Exec(`CREATE TABLE platform_schema(version INTEGER);INSERT INTO platform_schema VALUES(1)`); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			s, err := OpenStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var version int
			if err = s.DB.QueryRow(`SELECT version FROM platform_schema`).Scan(&version); err != nil || version != 2 {
				t.Fatal(version, err)
			}
			if err = s.migrate(); err != nil {
				t.Fatal("repeat migration", err)
			}
		})
	}
}
