package platform

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSQLiteRuntimeReopenRetainsBusinessDataAndIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var version, mode string
	if err = s.DB.QueryRow("SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "3.53.4" {
		t.Fatal("unexpected embedded SQLite runtime", version)
	}
	if err = s.DB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatal("WAL mode changed", mode, err)
	}
	ctx := context.Background()
	if err = s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 1, Name: "runtime-fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	projects, err := s.Projects(ctx)
	if err != nil || len(projects) != 1 || projects[0].Name != "runtime-fixture" {
		t.Fatal("business data lost after reopen", projects, err)
	}
	if _, err = s.Session(ctx, token); err != nil {
		t.Fatal("session lost after reopen", err)
	}
	var integrity string
	if err = s.DB.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("database integrity failed", integrity, err)
	}
}
