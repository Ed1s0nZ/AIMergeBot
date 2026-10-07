package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func repositoryProfile(t *testing.T, s *Store, actor int64, kind string, projects ...int) Integration {
	t.Helper()
	zero := int64(0)
	v, err := s.SaveIntegration(context.Background(), 0, actor, IntegrationInput{Integration: Integration{Name: "credential profile", Kind: kind, Enabled: true, ProjectIDs: projects, Events: []string{}, Frequency: "instant"}, ExpectedRevision: &zero, Credentials: &IntegrationCredentials{Endpoint: "https://api.example", Token: "fixture-private-token"}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func repositoryProjectInput(v Integration, request string) RepositoryProjectInput {
	revision := v.Revision
	return RepositoryProjectInput{RequestID: request, Name: "new repository", ExpectedIntegrationRevision: &revision, Provider: v.Kind, APIOrigin: "https://api.example", RemoteID: 7, FullName: "org/repo", IntegrationID: v.ID}
}
func TestRepositoryProjectBootstrapIdentityAndReplay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admin, err := s.CreateUser(ctx, "admin", "admin-long-password", "admin")
	if err != nil {
		t.Fatal(err)
	}
	github := repositoryProfile(t, s, admin.ID, "github")
	gitlab := repositoryProfile(t, s, admin.ID, "gitlab")
	for _, kind := range []string{"email", "feishu", "dingtalk", "wecom", "slack", "teams", "webhook", "jira", "linear"} {
		zero := int64(0)
		if _, err = s.SaveIntegration(ctx, 0, admin.ID, IntegrationInput{Integration: Integration{Name: "invalid", Kind: kind, Enabled: false, Frequency: "instant"}, ExpectedRevision: &zero}); !errors.Is(err, ErrIntegrationInput) {
			t.Fatal("empty scope broadened", kind, err)
		}
	}
	input := repositoryProjectInput(github, "11111111-1111-1111-1111-111111111111")
	created, err := s.CreateRepositoryProject(ctx, admin.ID, input)
	if err != nil || created.Project.ID != 1 || created.Project.ID == int(input.RemoteID) || created.Binding.Revision != 1 || created.IntegrationRevision != 2 || created.Replayed {
		t.Fatal(created, err)
	}
	replay, err := s.CreateRepositoryProject(ctx, admin.ID, input)
	if err != nil || !replay.Replayed || replay.Project.ID != created.Project.ID {
		t.Fatal(replay, err)
	}
	input.Name = "different"
	if _, err = s.CreateRepositoryProject(ctx, admin.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatal("request reuse accepted", err)
	}
	input = repositoryProjectInput(gitlab, "22222222-2222-2222-2222-222222222222")
	second, err := s.CreateRepositoryProject(ctx, admin.ID, input)
	if err != nil || second.Project.ID != 2 || second.Binding.RemoteID != created.Binding.RemoteID || second.Binding.Provider == created.Binding.Provider {
		t.Fatal(second, err)
	}
	integrations, err := s.Integrations(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range integrations {
		if len(v.ProjectIDs) != 1 || v.Revision != 2 || !v.HasSecret {
			t.Fatal(v)
		}
	}
	body, _ := json.Marshal(integrations)
	receipt, _ := json.Marshal(created)
	if strings.Contains(string(body)+string(receipt), "fixture-private-token") {
		t.Fatal("credential leaked")
	}
	if err = s.CollectNotifications(ctx, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"platform_project_members", "platform_runs", "platform_notification_deliveries"} {
		var count int
		if err = s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("unexpected side effect", table, count, err)
		}
	}
	if err = s.SaveLegacyProject(ctx, Project{ID: created.Project.ID, Name: "collision", Enabled: true}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	s = reopenPublicationGuardStore(t, s)
	replay, err = s.CreateRepositoryProject(ctx, admin.ID, repositoryProjectInput(github, "11111111-1111-1111-1111-111111111111"))
	if err != nil || !replay.Replayed || replay.Project.ID != created.Project.ID {
		t.Fatal("reopened receipt lost", replay, err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_projects`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
}

func TestRepositoryProjectConcurrentRequests(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(fmt.Sprint(sameKey), func(t *testing.T) {
			s, admin, _ := accessFixture(t)
			v := repositoryProfile(t, s, admin.ID, "github", 1, 2)
			var wg sync.WaitGroup
			results := make(chan RepositoryProjectReceipt, 8)
			failures := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					if sameKey {
						n = 0
					}
					input := repositoryProjectInput(v, fmt.Sprintf("00000000-0000-0000-0000-%012d", n))
					input.RemoteID = int64(n + 100)
					receipt, err := s.CreateRepositoryProject(context.Background(), admin.ID, input)
					if err != nil {
						failures <- err
					} else {
						results <- receipt
					}
				}(i)
			}
			wg.Wait()
			close(results)
			close(failures)
			success, replayed := 0, 0
			for r := range results {
				success++
				if r.Project.ID != 3 {
					t.Fatal(r)
				}
				if r.Replayed {
					replayed++
				}
			}
			failed := 0
			for err := range failures {
				failed++
				if !errors.Is(err, ErrConflict) {
					t.Fatal(err)
				}
			}
			if sameKey && (success != 8 || replayed != 7 || failed != 0) || !sameKey && (success != 1 || replayed != 0 || failed != 7) {
				t.Fatal(success, replayed, failed)
			}
			integrations, err := s.Integrations(context.Background(), admin.ID)
			if err != nil || len(integrations) != 1 || len(integrations[0].ProjectIDs) != 3 || integrations[0].ProjectIDs[0] != 1 || integrations[0].ProjectIDs[1] != 2 || integrations[0].ProjectIDs[2] != 3 || integrations[0].Revision != 2 {
				t.Fatal("scope lost", integrations, err)
			}
		})
	}
}

func TestRepositoryProjectRollbackAndInvalidConfiguration(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	v := repositoryProfile(t, s, admin.ID, "github", 1, 2)
	input := repositoryProjectInput(v, "33333333-3333-3333-3333-333333333333")
	if _, err := s.CreateRepositoryProject(ctx, member.ID, input); !errors.Is(err, ErrProjectPermission) {
		t.Fatal(err)
	}
	before, err := s.ProjectSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`CREATE TRIGGER abort_project_binding BEFORE INSERT ON platform_events WHEN NEW.action='repository.binding.updated' BEGIN SELECT RAISE(ABORT,'fixture abort'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateRepositoryProject(ctx, admin.ID, input); err == nil {
		t.Fatal("event failure ignored")
	}
	after, err := s.ProjectSyncStatus(ctx)
	if err != nil || after != before {
		t.Fatal("outbox leaked", before, after, err)
	}
	for _, table := range []string{"platform_repository_project_receipts", "platform_project_repositories", "platform_repository_binding_history"} {
		var count int
		if err = s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_projects`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	integrations, err := s.Integrations(ctx, admin.ID)
	if err != nil || integrations[0].Revision != 1 || len(integrations[0].ProjectIDs) != 2 {
		t.Fatal(integrations, err)
	}
	if _, err = s.DB.Exec(`DROP TRIGGER abort_project_binding`); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RepositoryProjectInput){
		func(p *RepositoryProjectInput) { p.RequestID = "bad" }, func(p *RepositoryProjectInput) { p.Name = " " }, func(p *RepositoryProjectInput) { p.ExpectedIntegrationRevision = nil }, func(p *RepositoryProjectInput) { n := int64(0); p.ExpectedIntegrationRevision = &n }, func(p *RepositoryProjectInput) { p.RemoteID = 0 }, func(p *RepositoryProjectInput) { p.APIOrigin = "https://user:secret@api.example" },
	} {
		invalid := input
		change(&invalid)
		if _, err = s.CreateRepositoryProject(ctx, admin.ID, invalid); !errors.Is(err, ErrRepositoryProjectInput) {
			t.Fatal(invalid, err)
		}
	}
	// Disabled integrations and missing or mismatched credentials cannot create a project.
	for _, update := range []string{`enabled=0`, `credentials='{"endpoint":"https://api.example"}'`, `credentials='{"endpoint":"https://wrong.example","token":"fixture"}'`} {
		if _, err = s.DB.Exec(`UPDATE platform_integrations SET `+update+` WHERE id=?`, v.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateRepositoryProject(ctx, admin.ID, input); !errors.Is(err, ErrConflict) {
			t.Fatal(update, err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_integrations SET enabled=1,credentials='{"endpoint":"https://api.example","token":"fixture-private-token"}' WHERE id=?`, v.ID); err != nil {
			t.Fatal(err)
		}
	}
	created, err := s.CreateRepositoryProject(ctx, admin.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	stale := repositoryProjectInput(v, "44444444-4444-4444-4444-444444444444")
	if _, err = s.CreateRepositoryProject(ctx, admin.ID, stale); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	fresh := stale
	n := int64(2)
	fresh.ExpectedIntegrationRevision = &n
	if _, err = s.CreateRepositoryProject(ctx, admin.ID, fresh); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate remote identity admitted", err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_repository_project_receipts SET receipt_json='corrupt'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateRepositoryProject(ctx, admin.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatal("corrupt receipt re-created project", err)
	}
	if created.Project.ID != 3 {
		t.Fatal(created)
	}
}

func TestRepositoryProjectCapacityAndActorIsolation(t *testing.T) {
	t.Run("full scope", func(t *testing.T) {
		s, admin, _ := accessFixture(t)
		ctx := context.Background()
		ids := []int{}
		for i := 1; i <= 100; i++ {
			if err := s.SaveProject(ctx, Project{ID: i, Name: fmt.Sprint(i), Enabled: true}); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, i)
		}
		v := repositoryProfile(t, s, admin.ID, "github", ids...)
		if _, err := s.CreateRepositoryProject(ctx, admin.ID, repositoryProjectInput(v, "55555555-5555-5555-5555-555555555555")); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		var count int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_projects`).Scan(&count); err != nil || count != 100 {
			t.Fatal(count, err)
		}
	})
	t.Run("ID overflow", func(t *testing.T) {
		s, admin, _ := accessFixture(t)
		v := repositoryProfile(t, s, admin.ID, "gitlab")
		if _, err := s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(9223372036854775807,'highest',1)`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateRepositoryProject(context.Background(), admin.ID, repositoryProjectInput(v, "66666666-6666-6666-6666-666666666666")); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})
	t.Run("actor key namespace and revoked actor", func(t *testing.T) {
		s, admin, _ := accessFixture(t)
		ctx := context.Background()
		other, err := s.CreateUser(ctx, "other-admin", "other-admin-password", "admin")
		if err != nil {
			t.Fatal(err)
		}
		v := repositoryProfile(t, s, admin.ID, "github")
		input := repositoryProjectInput(v, "77777777-7777-7777-7777-777777777777")
		first, err := s.CreateRepositoryProject(ctx, admin.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		revision := int64(2)
		otherInput := input
		otherInput.RemoteID = 8
		otherInput.ExpectedIntegrationRevision = &revision
		second, err := s.CreateRepositoryProject(ctx, other.ID, otherInput)
		if err != nil || second.Replayed || first.Project.ID == second.Project.ID {
			t.Fatal(second, err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, admin.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateRepositoryProject(ctx, admin.ID, input); !errors.Is(err, ErrCredentials) {
			t.Fatal("revoked actor replay admitted", err)
		}
	})
}
