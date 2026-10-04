package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStaleSettingsCannotOverwriteAnotherAdministrator(t *testing.T) {
	dir := t.TempDir()
	svc, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	first := svc.Snapshot()
	stale := svc.Snapshot()
	first.OpenAI.Model = "first-admin-model"
	if err = svc.Save(first); err != nil {
		t.Fatal(err)
	}
	stale.AuditWorkers = 3
	if err = svc.Save(stale); !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	if svc.Snapshot().OpenAI.Model != "first-admin-model" || svc.Snapshot().AuditWorkers != first.AuditWorkers {
		t.Fatal("stale write changed configuration")
	}
	reopened, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil || reopened.Snapshot().Revision != svc.Snapshot().Revision {
		t.Fatal("revision not durable")
	}
}
func TestProjectSyncFailureIsDurableAndRecoverable(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	svc, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 7, Name: "durable-project", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, svc); err == nil {
		t.Fatal("fixture did not fail atomic config replace")
	}
	allowImport, restoreErr := s.RestorePendingProjects(ctx, svc)
	if restoreErr == nil || allowImport {
		t.Fatal("failed sync allowed stale startup import")
	}
	var dirty bool
	var message string
	s.DB.QueryRow(`SELECT dirty,last_error FROM platform_project_sync WHERE id=1`).Scan(&dirty, &message)
	if !dirty || message == "" {
		t.Fatal("failed sync lost recovery intent")
	}
	projects, err := s.Projects(ctx)
	if err != nil || len(projects) != 1 || projects[0].Name != "durable-project" {
		t.Fatal("saved database project lost")
	}
	os.Remove(target)
	if err = s.SyncProjectConfig(ctx, svc); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT dirty FROM platform_project_sync WHERE id=1`).Scan(&dirty)
	if dirty || len(svc.Snapshot().Projects) != 1 || svc.Snapshot().Projects[0].ID != 7 {
		t.Fatal("recovery not acknowledged")
	}
}
