package platform

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

var errReloadPermission = errors.New("administrator permission changed")

type repositoryActivationStatus struct {
	State     string `json:"state"`
	Code      string `json:"code"`
	CanReload bool   `json:"can_reload"`
}

func (s *SettingsService) repositoryActivationStatus(ctx context.Context, revision uint64) *repositoryActivationStatus {
	a := s.activation.Load()
	if a == nil {
		return nil
	}
	status := &repositoryActivationStatus{State: "unavailable", Code: "credential_activation_unavailable"}
	if a.store == nil || a.store.repositoryCredentials.Load() != a {
		return status
	}
	v := a.view.Load()
	if v == nil {
		return status
	}
	if v.config.Revision != revision {
		status.Code = "settings_changed"
		return status
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := readLegacyRepositoryState(ctx, a.store.DB); err != nil {
		return status
	}
	if v.phase == "active" {
		if validateLegacyRepositoryState(ctx, a.store.DB, v.material, v.generation) != nil {
			return status
		}
		status.State, status.Code = "active", ""
	} else if v.phase == "pending" {
		status.State, status.Code = "pending", "credential_activation_pending"
	} else {
		return status
	}
	if s.activation.Load() != a || a.store.repositoryCredentials.Load() != a || a.view.Load() != v || s.current.Load().Revision != revision {
		return &repositoryActivationStatus{State: "unavailable", Code: "settings_changed"}
	}
	status.CanReload = true
	return status
}

func requireReloadAdministrator(ctx context.Context, q queryRower, actor int64) error {
	var valid bool
	if q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_users WHERE id=? AND role='admin' AND disabled=0)`, actor).Scan(&valid) != nil {
		return errCredentialActivationUnavailable
	}
	if !valid {
		return errReloadPermission
	}
	return nil
}

type repositoryReloadCommit struct {
	actor    int64
	revision uint64
}
type repositoryReloadCommitKey struct{}

func authorizeRepositoryReloadTx(ctx context.Context, tx *sql.Tx) error {
	if commit, ok := ctx.Value(repositoryReloadCommitKey{}).(repositoryReloadCommit); ok {
		return requireReloadAdministrator(ctx, tx, commit.actor)
	}
	return nil
}

func recordRepositoryReloadTx(ctx context.Context, tx *sql.Tx) error {
	commit, ok := ctx.Value(repositoryReloadCommitKey{}).(repositoryReloadCommit)
	if !ok {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'settings.reload.committed',?,?)`, commit.actor, "config.yaml revision:"+strconv.FormatUint(commit.revision, 10), now())
	if err != nil {
		return errCredentialActivationPending
	}
	return nil
}

// Only the existing server-bound path is read. File secrets are real values,
// including empty values; Save's blank-secret placeholders do not apply here.
func (s *SettingsService) reloadRepositoryCredentials(ctx context.Context, expected uint64, actor int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.activation.Load()
	if a == nil || a.store == nil || a.store.repositoryCredentials.Load() != a {
		return errCredentialActivationUnavailable
	}
	old := a.view.Load()
	if old == nil || (old.phase != "active" && old.phase != "pending") {
		return errCredentialActivationUnavailable
	}
	current := s.Snapshot()
	if expected == 0 || expected != current.Revision {
		return ErrSettingsConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := requireReloadAdministrator(ctx, a.store.DB, actor); err != nil {
		return err
	}
	state, err := readLegacyRepositoryState(ctx, a.store.DB)
	if err != nil {
		return errCredentialActivationUnavailable
	}
	if old.phase == "active" && validateLegacyRepositoryState(ctx, a.store.DB, old.material, old.generation) != nil {
		return errCredentialActivationUnavailable
	}
	next, err := readSettingsForReload(s.path)
	if err != nil {
		return err
	}
	next.Projects = current.Projects
	if validateSettings(next) != nil {
		return errReloadConfiguration
	}
	material, err := newLegacyRepositoryMaterial(next.GitLab.Token, next.GitLab.URL, a.options.proxy)
	if err != nil {
		return errReloadConfiguration
	}
	newest := max(current.Revision, next.Revision)
	fingerprint, err := material.fingerprint(state.key)
	if err != nil {
		return errCredentialActivationUnavailable
	}
	if newest == ^uint64(0) || old.epoch == ^uint64(0) || state.generation == 1<<63-1 && !hmac.Equal(fingerprint, state.fingerprint) {
		return errCredentialActivationExhausted
	}
	next.Revision = newest + 1
	raw, err := yaml.Marshal(next)
	if err != nil || len(raw) > 1<<20 {
		return errReloadConfiguration
	}
	if ctx.Err() != nil {
		return errCredentialActivationUnavailable
	}
	epoch := old.epoch
	changed := !hmac.Equal(old.material.encoded, material.encoded)
	if changed {
		epoch++
	}
	pending := *old
	pending.phase, pending.epoch = "pending", epoch
	if changed || old.phase == "pending" {
		a.view.Store(&pending)
	}
	replaced, err := a.options.persist(s.path, raw)
	if err != nil {
		if replaced {
			if !changed && old.phase == "active" {
				pending.epoch++
			}
			a.view.Store(&pending)
			return errCredentialActivationPending
		}
		if changed && old.phase == "active" {
			restored := *old
			restored.epoch = epoch
			a.view.Store(&restored)
		}
		return errCredentialActivationUnavailable
	}
	ctx = context.WithValue(ctx, repositoryReloadCommitKey{}, repositoryReloadCommit{actor: actor, revision: next.Revision})
	var expectedView *legacyCredentialView
	if old.phase == "active" {
		expectedView = old
	}
	if err = a.activateAndPublish(ctx, s, next, material, epoch, expectedView); err != nil {
		a.view.Store(&pending)
		return err
	}
	return nil
}
