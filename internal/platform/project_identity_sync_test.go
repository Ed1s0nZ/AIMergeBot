package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	legacy "pr_agent/internal"
)

func TestProjectIdentityImportAndContextGuard(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	bind := func() {
		t.Helper()
		if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example")); err != nil {
			t.Fatal(err)
		}
	}
	disabled := false
	imported, err := s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, Name: "original", Enabled: &disabled}, {ID: 3, Name: "new"}})
	if err != nil || len(imported) != 2 {
		t.Fatal(imported, err)
	}
	imported, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, Name: "renamed"}})
	if err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err = s.DB.QueryRow(`SELECT enabled FROM platform_projects WHERE id=1`).Scan(&enabled); err != nil || enabled {
		t.Fatal("missing enabled reset legacy state", enabled, err)
	}
	if err = s.DB.QueryRow(`SELECT enabled FROM platform_projects WHERE id=3`).Scan(&enabled); err != nil || !enabled {
		t.Fatal("new project default", enabled, err)
	}
	if err = s.SaveProject(ctx, Project{ID: 1, Name: "database-authority", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	contexts := []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("a", 40)}}
	if err = s.SaveContextRepositories(ctx, 1, contexts, admin.ID); err != nil {
		t.Fatal(err)
	}
	// A binding saved after project import must fence the later context import.
	imported, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, Name: "database-authority", ContextRepositories: []ContextRepository{}}})
	if err != nil {
		t.Fatal(err)
	}
	bind()
	if err = s.ImportContextRepositories(ctx, imported); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("late binding allowed context overwrite", err)
	}
	kept, err := s.ContextRepositories(ctx, 1)
	if err != nil || len(kept) != 1 || kept[0] != contexts[0] {
		t.Fatal(kept, err)
	}
	for _, batch := range [][]legacy.ProjectConfig{
		{{ID: 4, Name: "must-roll-back"}, {ID: 1, Name: "colliding-gitlab"}},
		{{ID: 4, Name: "must-roll-back"}, {ID: 4, Name: "duplicate"}},
		{{ID: 4, Name: "must-roll-back"}, {ID: 2, InternalProject: true}},
		{{ID: 4, Name: "must-roll-back"}, {ID: 999, InternalProject: true}},
	} {
		before, err := s.ProjectSyncStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.ImportConfiguredProjects(ctx, batch); !errors.Is(err, ErrConflict) {
			t.Fatal(batch, err)
		}
		var count int
		if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_projects WHERE id=4`).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial import", count, err)
		}
		after, err := s.ProjectSyncStatus(ctx)
		if err != nil || after != before {
			t.Fatal("rollback changed sync generation", before, after, err)
		}
	}
	imported, err = s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: 1, InternalProject: true, Name: "forged", Enabled: &disabled, ContextRepositories: []ContextRepository{}}})
	if err != nil || len(imported) != 0 {
		t.Fatal(imported, err)
	}
	if err = s.ImportContextRepositories(ctx, []legacy.ProjectConfig{{ID: 1, InternalProject: true, ContextRepositories: []ContextRepository{}}}); err != nil {
		t.Fatal(err)
	}
	var name string
	if err = s.DB.QueryRow(`SELECT name,enabled FROM platform_projects WHERE id=1`).Scan(&name, &enabled); err != nil || name != "database-authority" || !enabled {
		t.Fatal("internal reference overwrote DB", name, enabled, err)
	}
	if err = s.SaveLegacyProject(ctx, Project{ID: 1, Name: "wrong", Enabled: false}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestProjectIdentityConfigurationRecovery(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	target := filepath.Join(t.TempDir(), "config.yaml")
	settings, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
		t.Fatal(err)
	}
	before, err := s.ProjectSyncStatus(ctx)
	if err != nil || before.Pending {
		t.Fatal(before, err)
	}
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example")); err != nil {
		t.Fatal(err)
	}
	after, err := s.ProjectSyncStatus(ctx)
	if err != nil || !after.Pending || after.Generation != before.Generation+1 {
		t.Fatal("binding did not dirty sync atomically", before, after, err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.RestorePendingProjects(ctx, settings); err == nil || allowed {
		t.Fatal("failed sync allowed legacy import", allowed, err)
	}
	s = reopenPublicationGuardStore(t, s)
	if allowed, err := s.RestorePendingProjects(ctx, settings); err == nil || allowed {
		t.Fatal("restart lost sync intent", allowed, err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.RestorePendingProjects(ctx, settings); err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	reopened, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	projects := reopened.Snapshot().Projects
	if len(projects) != 2 {
		t.Fatal(projects)
	}
	foundInternal, foundLegacy := false, false
	for _, p := range projects {
		if p.ID == 1 {
			foundInternal = p.InternalProject
		}
		if p.ID == 2 {
			foundLegacy = !p.InternalProject
		}
	}
	if !foundInternal || !foundLegacy {
		t.Fatal("namespace not durable", projects)
	}
	raw, err := os.ReadFile(target)
	if err != nil || strings.Contains(string(raw), "fixture-private-token") || !strings.Contains(string(raw), "internal_project: true") {
		t.Fatal("invalid exported config", err)
	}
	legacyProjects, err := s.ImportConfiguredProjects(ctx, projects)
	if err != nil || len(legacyProjects) != 1 || legacyProjects[0].ID != 2 {
		t.Fatal("restart reinterpreted internal project", legacyProjects, err)
	}
	if err = s.ImportContextRepositories(ctx, legacyProjects); err != nil {
		t.Fatal(err)
	}
	// A corrupt identity can never be exported as a legacy project.
	if _, err = s.DB.Exec(`UPDATE platform_project_repositories SET binding_json='corrupt' WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); !errors.Is(err, ErrConflict) {
		t.Fatal("corrupt binding exported", err)
	}
	status, err := s.ProjectSyncStatus(ctx)
	if err != nil || !status.Pending || status.Error == "" {
		t.Fatal("lost failed sync evidence", status, err)
	}
	unchanged, err := os.ReadFile(target)
	if err != nil || string(unchanged) != string(raw) {
		t.Fatal("failed projection replaced file", err)
	}
}

func TestProjectIdentityConcurrentImportAndBinding(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	ids := []int{10, 11, 12, 13, 14, 15, 16, 17}
	for _, id := range ids {
		if err := s.SaveProject(ctx, Project{ID: id, Name: "original", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", ids...)
	for _, id := range ids {
		start := make(chan struct{})
		imported := make(chan error, 1)
		bound := make(chan error, 1)
		go func() {
			<-start
			_, err := s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: id + 100, Name: "atomic-companion"}, {ID: id, Name: "imported"}})
			imported <- err
		}()
		go func() {
			<-start
			binding := bindingValue(v, "github", "https://api.example")
			binding.RemoteID = int64(id)
			_, err := s.SaveRepositoryBinding(ctx, id, admin.ID, 0, binding)
			bound <- err
		}()
		close(start)
		importErr, bindErr := <-imported, <-bound
		if bindErr != nil || importErr != nil && !errors.Is(importErr, ErrConflict) {
			t.Fatal(importErr, bindErr)
		}
		var name string
		var count int
		if err := s.DB.QueryRow(`SELECT name FROM platform_projects WHERE id=?`, id).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_projects WHERE id=?`, id+100).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if importErr == nil && (name != "imported" || count != 1) || importErr != nil && (name != "original" || count != 0) {
			t.Fatal("interleaved import changed bound identity", name, count, importErr)
		}
		if _, err := s.ImportConfiguredProjects(ctx, []legacy.ProjectConfig{{ID: id, Name: "later-import"}}); !errors.Is(err, ErrConflict) {
			t.Fatal("post-bind import succeeded", err)
		}
	}
}

func TestProjectIdentityConcurrentBindingAndExport(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	target := filepath.Join(t.TempDir(), "config.yaml")
	settings, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	bound := make(chan error, 1)
	synced := make(chan error, 1)
	go func() {
		<-start
		for revision := int64(0); revision < 8; revision++ {
			if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, revision, bindingValue(v, "github", "https://api.example")); err != nil {
				bound <- err
				return
			}
		}
		bound <- nil
	}()
	go func() {
		<-start
		for n := 0; n < 8; n++ {
			if err := s.SyncProjectConfig(ctx, settings); err != nil {
				synced <- err
				return
			}
		}
		synced <- nil
	}()
	close(start)
	if err = <-bound; err != nil {
		t.Fatal(err)
	}
	if err = <-synced; err != nil {
		t.Fatal(err)
	}
	if err = s.SyncProjectConfig(ctx, settings); err != nil {
		t.Fatal(err)
	}
	status, err := s.ProjectSyncStatus(ctx)
	if err != nil || status.Pending {
		t.Fatal(status, err)
	}
	reopened, err := OpenSettings(target, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range reopened.Snapshot().Projects {
		if p.ID == 1 && !p.InternalProject {
			t.Fatal("clean outbox lost namespace", p)
		}
	}
	binding, err := s.RepositoryBinding(ctx, 1, admin.ID)
	if err != nil || binding.Revision != 8 {
		t.Fatal(binding, err)
	}
}
