package platform

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
	legacy "pr_agent/internal"
)

func reloadAdministrator(t *testing.T, s *Store) int64 {
	t.Helper()
	if err := s.Bootstrap(context.Background(), "admin", "admin-long-password"); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.DB.QueryRow(`SELECT id FROM platform_users WHERE username='admin'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func writeReloadFile(t *testing.T, svc *SettingsService, cfg Settings) {
	t.Helper()
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(svc.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func reloadEventCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_events WHERE action='settings.reload.committed'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCredentialReloadPendingPublicationAndReplay(t *testing.T) {
	ctx := context.Background()
	var fail atomic.Bool
	svc, s := activationFixture(t, credentialActivationOptions{beforePublish: func() error {
		if fail.Load() {
			return errors.New("PRIVATE publication failure")
		}
		return nil
	}})
	actor := reloadAdministrator(t, s)
	before := svc.Snapshot()
	file := before
	file.GitLab.Token = "PRIVATE-B"
	file.Revision = 9007199254740993
	file.Projects = []legacy.ProjectConfig{{ID: 999, Name: "must not import", ContextRepositories: []ContextRepository{{ProjectID: 999, SHA: "invalid self context"}}}}
	writeReloadFile(t, svc, file)
	if _, err := s.DB.Exec(`UPDATE platform_project_sync SET dirty=1,generation=generation+1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	syncBefore, _ := s.ProjectSyncStatus(ctx)
	fail.Store(true)
	if err := svc.reloadRepositoryCredentials(ctx, before.Revision, actor); !errors.Is(err, errCredentialActivationPending) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(svc.Snapshot(), before) || activationGeneration(t, s) != 2 || reloadEventCount(t, s) != 1 {
		t.Fatal("commit/publication boundary lost")
	}
	status := svc.repositoryActivationStatus(ctx, before.Revision)
	if status.State != "pending" || !status.CanReload {
		t.Fatal(status)
	}
	fail.Store(false)
	if err := svc.reloadRepositoryCredentials(ctx, before.Revision, actor); err != nil {
		t.Fatal(err)
	}
	after := svc.Snapshot()
	if after.Revision != 9007199254740995 || after.GitLab.Token != "PRIVATE-B" || !reflect.DeepEqual(after.Projects, before.Projects) || activationGeneration(t, s) != 2 || reloadEventCount(t, s) != 2 {
		t.Fatal("wrong recovered configuration")
	}
	syncAfter, _ := s.ProjectSyncStatus(ctx)
	if syncBefore != syncAfter {
		t.Fatal("reload modified project outbox")
	}
	rawBefore, _ := os.ReadFile(svc.path)
	if err := svc.reloadRepositoryCredentials(ctx, before.Revision, actor); !errors.Is(err, ErrSettingsConflict) {
		t.Fatal(err)
	}
	rawAfter, _ := os.ReadFile(svc.path)
	if string(rawAfter) != string(rawBefore) || reloadEventCount(t, s) != 2 {
		t.Fatal("lost-response replay wrote state")
	}
	var target string
	if err := s.DB.QueryRow(`SELECT target FROM platform_events WHERE action='settings.reload.committed' ORDER BY id DESC LIMIT 1`).Scan(&target); err != nil || target != "config.yaml revision:9007199254740995" {
		t.Fatal(target, err)
	}
	status = svc.repositoryActivationStatus(ctx, after.Revision)
	if status.State != "active" || status.Code != "" || !status.CanReload {
		t.Fatal(status)
	}
	if status = svc.repositoryActivationStatus(ctx, before.Revision); status.Code != "settings_changed" || status.CanReload {
		t.Fatal(status)
	}
}

func TestCredentialReloadFileSecretsAndGeneration(t *testing.T) {
	svc, s := activationFixture(t, credentialActivationOptions{})
	actor := reloadAdministrator(t, s)
	ctx := context.Background()
	for i, token := range []string{"PRIVATE-A", "PRIVATE-B", "PRIVATE-A", ""} {
		before := svc.Snapshot()
		cfg := before
		cfg.Revision = 1 // stale file counter must never move the global version back.
		cfg.GitLab.Token = token
		cfg.OpenAI.APIKey = ""
		cfg.WebhookToken = ""
		writeReloadFile(t, svc, cfg)
		if err := svc.reloadRepositoryCredentials(ctx, before.Revision, actor); err != nil {
			t.Fatal(err)
		}
		after := svc.Snapshot()
		if after.Revision != before.Revision+1 || after.GitLab.Token != token || after.OpenAI.APIKey != "" || after.WebhookToken != "" || activationGeneration(t, s) != int64(i+1) {
			t.Fatal("secret retention or nonmonotonic generation", i)
		}
	}
}

func TestCredentialReloadFailureBoundaries(t *testing.T) {
	for _, mode := range []string{"before_replace", "after_replace", "event", "revoked", "state"} {
		t.Run(mode, func(t *testing.T) {
			var inject atomic.Bool
			var store *Store
			var actor int64
			svc, s := activationFixture(t, credentialActivationOptions{persist: func(path string, raw []byte) (bool, error) {
				if inject.Load() && mode == "before_replace" {
					return false, errors.New("PRIVATE filesystem")
				}
				replaced, err := persistSettingsFile(path, raw, true)
				if inject.Load() && mode == "after_replace" {
					return true, errors.New("PRIVATE durability")
				}
				if inject.Load() && mode == "revoked" {
					_, err = store.DB.Exec(`UPDATE platform_users SET role='member' WHERE id=?`, actor)
				}
				return replaced, err
			}})
			store = s
			actor = reloadAdministrator(t, s)
			before := svc.Snapshot()
			run, _, err := s.Enqueue(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			file := before
			file.GitLab.Token = "PRIVATE-B"
			writeReloadFile(t, svc, file)
			rawBefore, _ := os.ReadFile(svc.path)
			if mode == "event" {
				if _, err := s.DB.Exec(`CREATE TRIGGER fail_reload_event BEFORE INSERT ON platform_events WHEN NEW.action='settings.reload.committed' BEGIN SELECT RAISE(ABORT,'PRIVATE SQL'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "state" {
				if _, err := s.DB.Exec(`DELETE FROM platform_legacy_repository_state`); err != nil {
					t.Fatal(err)
				}
			}
			inject.Store(true)
			err = svc.reloadRepositoryCredentials(context.Background(), before.Revision, actor)
			want := errCredentialActivationPending
			if mode == "before_replace" || mode == "state" {
				want = errCredentialActivationUnavailable
			}
			if mode == "revoked" {
				want = errReloadPermission
			}
			if !errors.Is(err, want) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(svc.Snapshot(), before) || reloadEventCount(t, s) != 0 {
				t.Fatal("failed operation published configuration/event")
			}
			if mode != "state" && activationGeneration(t, s) != 1 {
				t.Fatal("failed transaction changed generation")
			}
			var runState string
			if err := s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, run).Scan(&runState); err != nil || runState != "pending" {
				t.Fatal("failed transaction cancelled run", runState, err)
			}
			rawAfter, _ := os.ReadFile(svc.path)
			if (mode == "before_replace" || mode == "state") && string(rawBefore) != string(rawAfter) {
				t.Fatal("preflight failure wrote file")
			}
			if mode == "before_replace" && !svc.activation.Load().active() {
				t.Fatal("definite pre-replace failure closed active gate")
			}
			if mode == "event" || mode == "after_replace" || mode == "revoked" {
				if svc.activation.Load().active() {
					t.Fatal("possible replace failure left active")
				}
				inject.Store(false)
				err = nil
				if mode == "event" {
					_, err = s.DB.Exec(`DROP TRIGGER fail_reload_event`)
				}
				if mode == "revoked" {
					_, err = s.DB.Exec(`UPDATE platform_users SET role='admin' WHERE id=?`, actor)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = svc.reloadRepositoryCredentials(context.Background(), before.Revision, actor); err != nil || activationGeneration(t, s) != 2 || reloadEventCount(t, s) != 1 {
					t.Fatal("explicit recovery failed", err)
				}
				if err := s.DB.QueryRow(`SELECT status FROM platform_runs WHERE id=?`, run).Scan(&runState); err != nil || runState != "cancelled" {
					t.Fatal("committed recovery did not cancel old run", runState, err)
				}
			}
		})
	}
}

func TestCredentialReloadRejectsVersionsAndInvalidFilesWithoutWrites(t *testing.T) {
	for _, mode := range []string{"max_memory", "max_file", "bad_yaml", "nonadmin", "unbound", "missing", "directory", "generation_max"} {
		t.Run(mode, func(t *testing.T) {
			svc, s := activationFixture(t, credentialActivationOptions{})
			actor := reloadAdministrator(t, s)
			before := svc.Snapshot()
			cfg := before
			if mode == "max_memory" {
				before.Revision = ^uint64(0)
				svc.current.Store(&before)
			}
			if mode == "max_file" {
				cfg.Revision = ^uint64(0)
			}
			if mode == "generation_max" {
				_, err := s.DB.Exec(`UPDATE platform_legacy_repository_state SET generation=?`, int64(1<<63-1))
				if err != nil {
					t.Fatal(err)
				}
				v := *svc.activation.Load().view.Load()
				v.phase = "pending"
				svc.activation.Load().view.Store(&v)
				cfg.GitLab.Token = "PRIVATE-B"
			}
			writeReloadFile(t, svc, cfg)
			if mode == "bad_yaml" {
				if err := os.WriteFile(svc.path, []byte("PRIVATE malformed: ["), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "nonadmin" {
				actor = 999
			}
			if mode == "unbound" {
				svc.activation.Store(nil)
			}
			if mode == "missing" || mode == "directory" {
				if err := os.Remove(svc.path); err != nil {
					t.Fatal(err)
				}
				if mode == "directory" {
					if err := os.Mkdir(svc.path, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			rawBefore, _ := os.ReadFile(svc.path)
			err := svc.reloadRepositoryCredentials(context.Background(), before.Revision, actor)
			if err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal(err)
			}
			rawAfter, _ := os.ReadFile(svc.path)
			if string(rawBefore) != string(rawAfter) || reloadEventCount(t, s) != 0 {
				t.Fatal("rejection wrote file/event")
			}
		})
	}
}

func TestSettingsReloadStrictYAML(t *testing.T) {
	for _, raw := range []string{
		"", "[]", "config_revision: 01", "config_revision: 0x10", "config_revision: +1", "config_revision: -1", "config_revision: 1.0", "config_revision: '1'", "config_revision: 18446744073709551616", "config_revision: 1\nconfig_revision: 2", "listen: a\nlisten: b", "listen: &x foo", "listen: *x", "x: {<<: 1}", "unknown_field: true", "listen: foo\n---\nlisten: bar", "listen: \xff", "listen: " + strings.Repeat("x", 1<<20), "projects: " + strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70),
	} {
		if _, err := decodeSettingsForReload([]byte(raw)); !errors.Is(err, errReloadConfiguration) {
			t.Errorf("accepted invalid YAML of length %d", len(raw))
		}
	}
	for _, revision := range []uint64{0, 1, 9007199254740993, ^uint64(0)} {
		cfg, err := decodeSettingsForReload([]byte("config_revision: " + strconv.FormatUint(revision, 10)))
		if err != nil || cfg.Revision != max(revision, 1) || cfg.Listen != ":8080" || !cfg.GitAudit.Enabled || !cfg.VerifyFindings {
			t.Fatal("lost revision/defaults", err)
		}
	}
	if _, err := decodeSettingsForReload([]byte("whitelist_extensions:\n" + strings.Repeat("- x\n", 100001))); !errors.Is(err, errReloadConfiguration) {
		t.Fatal("unbounded YAML nodes accepted")
	}
}

func TestCredentialReloadStatusAndUnchangedReaders(t *testing.T) {
	ctx := context.Background()
	var inspect func()
	svc, s := activationFixture(t, credentialActivationOptions{beforePublish: func() error {
		if inspect != nil {
			inspect()
		}
		return nil
	}})
	actor := reloadAdministrator(t, s)
	op, _, err := svc.legacyOperationContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := svc.Snapshot()
	file := before
	file.OpenAI.Model = "reloaded-model"
	writeReloadFile(t, svc, file)
	inspect = func() {
		status := svc.repositoryActivationStatus(ctx, before.Revision)
		if status.State != "active" || !status.CanReload || checkRepositoryFence(op) != nil || svc.Snapshot().OpenAI.Model != before.OpenAI.Model {
			t.Fatal("model-only reload disturbed readers")
		}
	}
	if err = svc.reloadRepositoryCredentials(ctx, before.Revision, actor); err != nil || checkRepositoryFence(op) != nil {
		t.Fatal(err)
	}
	before = svc.Snapshot()
	file = before
	file.GitLab.Token = "PRIVATE-B"
	writeReloadFile(t, svc, file)
	inspect = func() {
		status := svc.repositoryActivationStatus(ctx, before.Revision)
		if status.State != "pending" || !status.CanReload || checkRepositoryFence(op) == nil {
			t.Fatal("changed credentials did not close old readers")
		}
	}
	if err = svc.reloadRepositoryCredentials(ctx, before.Revision, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_legacy_repository_state`); err != nil {
		t.Fatal(err)
	}
	status := svc.repositoryActivationStatus(ctx, svc.Snapshot().Revision)
	if status.State != "unavailable" || status.CanReload || status.Code != "credential_activation_unavailable" {
		t.Fatal(status)
	}
}
