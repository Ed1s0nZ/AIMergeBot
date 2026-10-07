package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func bindingIntegration(t *testing.T, s *Store, admin int64, kind, origin string, projects ...int) Integration {
	t.Helper()
	zero := int64(0)
	v, err := s.SaveIntegration(context.Background(), 0, admin, IntegrationInput{Integration: Integration{Name: "repository fixture", Kind: kind, Enabled: true, ProjectIDs: projects, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: origin, Token: "fixture-private-token"}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func bindingValue(v Integration, provider, origin string) RepositoryBinding {
	return RepositoryBinding{Provider: provider, APIOrigin: origin, RemoteID: 7, FullName: "org/repo", IntegrationID: v.ID}
}

func TestRepositoryBindingIdentityPermissionsAndHistory(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
	p := bindingValue(v, "github", "https://API.EXAMPLE:443/")
	old, err := s.RepositoryBinding(ctx, 1, admin.ID)
	if err != nil || old.Revision != 0 {
		t.Fatal(old, err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 1, member.ID, 0, p); err == nil {
		t.Fatal("member wrote binding")
	}
	saved, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p)
	if err != nil || saved.Revision != 1 || saved.APIOrigin != "https://api.example" {
		t.Fatal(saved, err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 2, admin.ID, 0, p); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate remote identity", err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p); !errors.Is(err, ErrConflict) {
		t.Fatal("stale write", err)
	}
	if _, err = s.RepositoryBinding(ctx, 1, member.ID); err == nil {
		t.Fatal("unauthorized read")
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	read, err := s.RepositoryBinding(ctx, 1, member.ID)
	if err != nil || read != saved {
		t.Fatal(read, err)
	}
	if _, err = s.RepositoryBinding(ctx, 2, member.ID); err == nil {
		t.Fatal("cross project read")
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RepositoryBinding(ctx, 1, member.ID); err == nil {
		t.Fatal("revoked read")
	}
	// The same remote number belongs to independent platform namespaces.
	lab := bindingIntegration(t, s, admin.ID, "gitlab", "https://api.example", 2)
	if _, err = s.SaveRepositoryBinding(ctx, 2, admin.ID, 0, bindingValue(lab, "gitlab", "https://api.example")); err != nil {
		t.Fatal(err)
	}
	// Another origin is also independent, without changing project ACLs.
	other := bindingIntegration(t, s, admin.ID, "github", "https://other.example/api/v3", 2)
	if _, err = s.SaveRepositoryBinding(ctx, 2, admin.ID, 1, bindingValue(other, "github", "https://other.example/api/v3")); err != nil {
		t.Fatal(err)
	}
	var count int
	var raw string
	if err = s.DB.QueryRow(`SELECT COUNT(*),MAX(binding_json) FROM platform_repository_binding_history WHERE project_id=2`).Scan(&count, &raw); err != nil || count != 2 || strings.Contains(raw, "fixture-private-token") {
		t.Fatal(count, raw, err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_project_members WHERE user_id=?`, member.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("binding granted ACL", count, err)
	}
}

func TestRepositoryBindingRejectsInvalidConfiguration(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	base := bindingValue(v, "github", "https://api.example")
	mutations := []func(*RepositoryBinding){
		func(p *RepositoryBinding) { p.Provider = "other" }, func(p *RepositoryBinding) { p.RemoteID = 0 }, func(p *RepositoryBinding) { p.IntegrationID = 0 },
		func(p *RepositoryBinding) { p.FullName = "org/repo/other" }, func(p *RepositoryBinding) { p.FullName = "org/.." }, func(p *RepositoryBinding) { p.FullName = "org//repo" }, func(p *RepositoryBinding) { p.FullName = "org/white space" }, func(p *RepositoryBinding) { p.FullName = strings.Repeat("a", 256) },
		func(p *RepositoryBinding) { p.APIOrigin = "http://api.example" }, func(p *RepositoryBinding) { p.APIOrigin = "https://user:secret@api.example" }, func(p *RepositoryBinding) { p.APIOrigin = "https://api.example?secret=foo" }, func(p *RepositoryBinding) { p.APIOrigin = "https://api.example#secret" }, func(p *RepositoryBinding) { p.APIOrigin = "https://api.example/other" }, func(p *RepositoryBinding) { p.APIOrigin = "https://wrong.example" }, func(p *RepositoryBinding) { p.APIOrigin = "https://api.example/%61pi/v3" },
	}
	for i, mutate := range mutations {
		p := base
		mutate(&p)
		if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p); err == nil {
			t.Fatalf("invalid mutation %d", i)
		}
	}
	if _, err := s.SaveRepositoryBinding(ctx, 2, admin.ID, 0, base); err == nil {
		t.Fatal("integration scope bypass")
	}
	if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, -1, base); err == nil {
		t.Fatal("negative revision")
	}
	if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 1<<63-1, base); err == nil {
		t.Fatal("revision overflow")
	}
	for _, query := range []string{`UPDATE platform_integrations SET enabled=0`, `UPDATE platform_integrations SET enabled=1,kind='webhook'`, `UPDATE platform_integrations SET kind='github',credentials='{"endpoint":"https://api.example","token":""}'`} {
		if _, err := s.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, base); err == nil {
			t.Fatal("invalid integration accepted")
		}
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestRepositoryBindingConcurrentCASAndAtomicHistory(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	p := bindingValue(v, "github", "https://api.example")
	var wg sync.WaitGroup
	var success atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p); err == nil {
				success.Add(1)
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatal(success.Load())
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_binding_event BEFORE INSERT ON platform_events WHEN NEW.action='repository.binding.updated' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	p.RemoteID = 8
	if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 1, p); err == nil {
		t.Fatal("event failure ignored")
	}
	old, err := s.RepositoryBinding(ctx, 1, admin.ID)
	if err != nil || old.Revision != 1 || old.RemoteID != 7 {
		t.Fatal(old, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = s.DB.Exec(`DROP TRIGGER fail_binding_event`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 1, p); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_project_repositories SET binding_json='{}' WHERE project_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RepositoryBinding(ctx, 1, admin.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("corruption fell back to legacy", err)
	}
}

func TestRepositoryBindingPersistsWithoutLegacyBackfill(t *testing.T) {
	ctx := context.Background()
	dbpath := filepath.Join(t.TempDir(), "binding.db")
	s, err := OpenStore(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.CreateUser(ctx, "admin", "long-fixture-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2} {
		if err = s.SaveProject(ctx, Project{ID: id, Name: "fixture", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	saved, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	read, err := s.RepositoryBinding(ctx, 1, admin.ID)
	if err != nil || read != saved {
		t.Fatal(read, err)
	}
	legacy, err := s.RepositoryBinding(ctx, 2, admin.ID)
	if err != nil || legacy.Revision != 0 {
		t.Fatal(legacy, err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, saved); !errors.Is(err, ErrConflict) {
		t.Fatal("restart allowed stale overwrite", err)
	}
}

func TestRepositoryBindingRejectsDisabledProjectAndCorruptEvidence(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	p := bindingValue(v, "github", "https://api.example")
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p); err == nil {
		t.Fatal("disabled target accepted")
	}
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{strings.Repeat("x", 4097), `{"revision":1,"provider":"gitlab","api_origin":"https://api.example","remote_id":7,"full_name":"org/repo","integration_id":1}`} {
		if _, err := s.DB.Exec(`UPDATE platform_project_repositories SET binding_json=? WHERE project_id=1`, raw); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RepositoryBinding(ctx, 1, admin.ID); !errors.Is(err, ErrConflict) {
			t.Fatal("corrupt binding accepted", err)
		}
		if _, err := s.SaveRepositoryBinding(ctx, 1, admin.ID, 1, saved); !errors.Is(err, ErrConflict) {
			t.Fatal("corrupt binding overwritten", err)
		}
	}
}

func TestRepositoryBindingMigrationFromUnboundSchema(t *testing.T) {
	ctx := context.Background()
	dbpath := filepath.Join(t.TempDir(), "legacy.db")
	s, err := OpenStore(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.CreateUser(ctx, "admin", "long-fixture-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveProject(ctx, Project{ID: 1, Name: "legacy", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// These are the only objects added by this migration. Removing them represents
	// the prior schema without inventing current repository bindings for old runs.
	for _, query := range []string{`DROP TABLE platform_repository_binding_history`, `DROP TABLE platform_project_repositories`} {
		if _, err = s.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.RepositoryBinding(ctx, 1, admin.ID)
	if err != nil || p.Revision != 0 {
		t.Fatal(p, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_repository_binding_history`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1)
	if _, err = s.SaveRepositoryBinding(ctx, 1, admin.ID, 0, bindingValue(v, "github", "https://api.example")); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryBindingCanonicalOrigin(t *testing.T) {
	for _, raw := range []string{"https://api.example#", "https://api.example?", "https://api.example:0", "https://api.example:65536", "https://api.example/\u0085", "https://api.example/api/v3//", "https://api.example/%2f"} {
		if _, err := canonicalRepositoryAPIOrigin("github", raw); err == nil {
			t.Fatalf("invalid API origin %q", raw)
		}
	}
	for raw, want := range map[string]string{"https://API.EXAMPLE:0443/": "https://api.example", "https://enterprise.example:08443/api/v3/": "https://enterprise.example:8443/api/v3"} {
		got, err := canonicalRepositoryAPIOrigin("github", raw)
		if err != nil || got != want {
			t.Fatal(raw, got, err)
		}
	}
}

func TestRepositoryBindingAllowsGitHubDotRepository(t *testing.T) {
	p := RepositoryBinding{Provider: "github", APIOrigin: "https://api.example", RemoteID: 1, FullName: "org/.github", IntegrationID: 1}
	if _, err := validateRepositoryBinding(p); err != nil {
		t.Fatal("valid special repository rejected", err)
	}
	p.FullName = "org_name/repo"
	if _, err := validateRepositoryBinding(p); err == nil {
		t.Fatal("invalid GitHub owner accepted")
	}
	p.Provider = "gitlab"
	p.FullName = "group/sub_group/repo"
	if _, err := validateRepositoryBinding(p); err != nil {
		t.Fatal("GitLab nested group rejected", err)
	}
}
