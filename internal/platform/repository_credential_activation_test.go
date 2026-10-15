package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func activationFixture(t *testing.T, options credentialActivationOptions) (*SettingsService, *Store) {
	t.Helper()
	s := testStore(t)
	svc, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.GitLab.URL, cfg.GitLab.Token = "https://gitlab.example/prefix", "PRIVATE-A"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	options.proxy = noCredentialProxy
	if err = svc.bindRepositoryCredentialActivation(context.Background(), s, options); err != nil {
		t.Fatal(err)
	}
	return svc, s
}

func activationGeneration(t *testing.T, s *Store) int64 {
	t.Helper()
	state, err := readLegacyRepositoryState(context.Background(), s.DB)
	if err != nil {
		t.Fatal(err)
	}
	return state.generation
}

func TestCredentialActivationSaveModelProjectsAndRotation(t *testing.T) {
	var interrupted []int64
	svc, s := activationFixture(t, credentialActivationOptions{interrupt: func(ids []int64) { interrupted = append(interrupted, ids...) }})
	ctx := context.Background()
	descriptor, err := svc.captureLegacyCredential(ctx)
	if err != nil || activationGeneration(t, s) != 1 {
		t.Fatal("startup not active", err)
	}
	model := svc.Snapshot()
	model.OpenAI.Model = "another-model"
	if err = svc.Save(model); err != nil || activationGeneration(t, s) != 1 || descriptor.validate(ctx) != nil {
		t.Fatal("model save rotated or revoked credentials", err)
	}
	if err = svc.SyncProjects([]Project{{ID: 8, Name: "outbox", Enabled: true}}); err != nil || activationGeneration(t, s) != 1 || descriptor.validate(ctx) != nil {
		t.Fatal("project sync rotated credentials", err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	run, _, err := s.Enqueue(ctx, snap, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"summary":"retained","findings":[]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	next := svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	if err = svc.Save(next); err != nil || activationGeneration(t, s) != 2 {
		t.Fatal("credential save failed", err)
	}
	if descriptor.validate(ctx) == nil {
		t.Fatal("old descriptor survived credential change")
	}
	var status, result string
	if err = s.DB.QueryRow(`SELECT status,result_json FROM platform_runs WHERE id=?`, run).Scan(&status, &result); err != nil || status != "cancelled" || !strings.Contains(result, "retained") || !reflect.DeepEqual(interrupted, []int64{run}) {
		t.Fatal("old run not atomically cancelled or evidence lost", status, err)
	}
	if _, err = svc.captureLegacyCredential(ctx); err != nil {
		t.Fatal("new descriptor unavailable", err)
	}
	next = svc.Snapshot()
	next.GitLab.Token = "PRIVATE-A"
	if err = svc.Save(next); err != nil || activationGeneration(t, s) != 3 {
		t.Fatal("credential ABA reused generation", err)
	}
}

func TestCredentialActivationPersistenceFaults(t *testing.T) {
	for _, mode := range []string{"before_replace", "after_replace", "db_cancel", "after_commit", "cross_connection"} {
		t.Run(mode, func(t *testing.T) {
			var publishCalls atomic.Int32
			var store *Store
			options := credentialActivationOptions{beforePublish: func() error {
				if publishCalls.Add(1) > 1 && mode == "after_commit" {
					return errors.New("PRIVATE-PUBLISH-ERROR")
				}
				return nil
			}}
			options.persist = func(path string, raw []byte) (bool, error) {
				if mode == "before_replace" {
					return false, errors.New("PRIVATE-FILE-ERROR")
				}
				replaced, err := persistSettingsFile(path, raw, true)
				if err != nil {
					return replaced, err
				}
				if mode == "after_replace" {
					return true, errors.New("PRIVATE-DIRECTORY-SYNC-ERROR")
				}
				if mode == "cross_connection" {
					if _, _, err := activateCredentialFixture(context.Background(), store, credentialMaterialFixture(t, "PRIVATE-OTHER")); err != nil {
						t.Fatal(err)
					}
				}
				return true, nil
			}
			svc, s := activationFixture(t, options)
			store = s
			ctx := context.Background()
			old, err := svc.captureLegacyCredential(ctx)
			if err != nil {
				t.Fatal(err)
			}
			run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "db_cancel" {
				if _, err := s.DB.Exec(`CREATE TRIGGER stop_credential_cancel BEFORE UPDATE OF status ON platform_runs BEGIN SELECT RAISE(ABORT,'PRIVATE-CANCEL-ERROR');END`); err != nil {
					t.Fatal(err)
				}
			}
			before := svc.Snapshot()
			next := before
			next.GitLab.Token, next.OpenAI.Model = "PRIVATE-B", "new-model"
			if err = svc.Save(next); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("fault accepted or leaked", err)
			}
			if !reflect.DeepEqual(svc.Snapshot(), before) {
				t.Fatal("partial settings published")
			}
			var status string
			if err := s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, run).Scan(&status); err != nil {
				t.Fatal(err)
			}
			wantGeneration, wantStatus := int64(1), "pending"
			if mode == "after_commit" {
				wantGeneration, wantStatus = 2, "cancelled"
			}
			if mode == "cross_connection" {
				wantGeneration = 2
			}
			if activationGeneration(t, s) != wantGeneration || status != wantStatus {
				t.Fatal("activation/cancellation transaction split", status)
			}
			if old.validate(ctx) == nil {
				t.Fatal("old in-flight descriptor survived pending epoch")
			}
			raw, err := os.ReadFile(svc.path)
			if err != nil {
				t.Fatal(err)
			}
			var file Settings
			if err = yaml.Unmarshal(raw, &file); err != nil {
				t.Fatal(err)
			}
			if mode == "before_replace" {
				if file.GitLab.Token != before.GitLab.Token || !svc.activation.Load().active() {
					t.Fatal("definite pre-replace failure changed active credential")
				}
				if _, err = svc.captureLegacyCredential(ctx); err != nil {
					t.Fatal("fresh descriptor unavailable after definite failure", err)
				}
			} else {
				if file.GitLab.Token != next.GitLab.Token || svc.activation.Load().active() {
					t.Fatal("saved file lost or gate reopened")
				}
				if err = svc.Save(before); !errors.Is(err, errCredentialActivationPending) {
					t.Fatal("pending ordinary save accepted", err)
				}
				if err = svc.SyncProjects([]Project{{ID: 2, Name: "must-not-overwrite", Enabled: true}}); !errors.Is(err, errCredentialActivationPending) {
					t.Fatal("pending outbox save accepted", err)
				}
				after, _ := os.ReadFile(svc.path)
				if string(after) != string(raw) {
					t.Fatal("pending save overwrote durable config")
				}
				if mode == "db_cancel" {
					if _, err := s.DB.Exec(`DROP TRIGGER stop_credential_cancel`); err != nil {
						t.Fatal(err)
					}
				}
				restarted, err := OpenSettings(svc.path, "../../config.example.yaml")
				if err != nil {
					t.Fatal(err)
				}
				if err = restarted.bindRepositoryCredentialActivation(ctx, s, credentialActivationOptions{proxy: noCredentialProxy}); err != nil {
					t.Fatal("startup recovery failed", err)
				}
				want := int64(2)
				if mode == "cross_connection" {
					want = 3
				}
				if activationGeneration(t, s) != want || restarted.Snapshot().GitLab.Token != "PRIVATE-B" || restarted.Snapshot().OpenAI.Model != "new-model" {
					t.Fatal("restart duplicated activation or published partial configuration")
				}
				if _, err = restarted.captureLegacyCredential(ctx); err != nil {
					t.Fatal("recovered descriptor unavailable", err)
				}
			}
		})
	}
}

func TestCredentialActivationPendingReadersDoNotWaitForSettingsWriter(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	svc, _ := activationFixture(t, credentialActivationOptions{persist: func(path string, raw []byte) (bool, error) {
		close(entered)
		<-release
		return persistSettingsFile(path, raw, true)
	}})
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	old, err := svc.captureLegacyCredential(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := svc.Snapshot()
	next.GitLab.Token = "PRIVATE-B"
	var saveErr error
	go func() { saveErr = svc.Save(next); close(finished) }()
	defer func() { unblock(); <-finished }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("writer did not reach persistence")
	}
	if _, err = svc.captureLegacyCredential(ctx); err == nil {
		t.Fatal("pending reader captured credentials")
	}
	if old.validate(ctx) == nil || svc.Snapshot().GitLab.Token != "PRIVATE-A" {
		t.Fatal("pending leaked new config or authorized old operation")
	}
	unblock()
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("writer did not finish")
	}
	if saveErr != nil {
		t.Fatal(saveErr)
	}
}

func TestCredentialActivationCancelsOnlyLegacyCredentialScopes(t *testing.T) {
	for _, mode := range []string{"integration", "target", "source", "context"} {
		t.Run(mode, func(t *testing.T) {
			svc, s := activationFixture(t, credentialActivationOptions{})
			scope := repositoryScopeFixture()
			legacy := RepositoryReference{ProjectID: 3, IdentityRevision: 1, Provider: "gitlab", APIOrigin: "https://gitlab.example", RemoteID: 3, FullName: "org/legacy", Credential: RepositoryCredential{Kind: "legacy_gitlab", LegacyRepositoryRevision: 1}}
			switch mode {
			case "target", "source":
				scope.Observation.Provider = "gitlab"
				scope.Target.Provider, scope.Source.Provider = "gitlab", "gitlab"
				scope.Target.APIOrigin, scope.Source.APIOrigin = legacy.APIOrigin, legacy.APIOrigin
				if mode == "target" {
					scope.Target = legacy
				} else {
					scope.Source = legacy
				}
			case "context":
				scope.Context = []RepositoryContextReference{{Repository: legacy, SHA: scope.Observation.HeadSHA}}
			}
			snap := repositoryScopeSnapshot(scope)
			if err := validateRepositoryScopeSnapshot(snap); err != nil {
				t.Fatal("invalid scope fixture", err)
			}
			policy := snap.AuditPolicy
			snap.AuditPolicy = nil
			ctx := context.Background()
			id, _, err := s.Enqueue(ctx, snap, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			// A valid frozen future scope is injected only in the temporary store;
			// production admission continues to reject unsupported native scopes.
			injectRepositoryScope(t, s, id, policy)
			next := svc.Snapshot()
			next.GitLab.Token = "PRIVATE-B"
			if err = svc.Save(next); err != nil {
				t.Fatal(err)
			}
			var status string
			want := "cancelled"
			if mode == "integration" {
				want = "pending"
			}
			if err = s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, id).Scan(&status); err != nil || status != want {
				t.Fatal("credential ownership cancellation mismatch", status, err)
			}
		})
	}
}
