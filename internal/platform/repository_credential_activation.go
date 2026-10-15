package platform

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

var errCredentialActivationPending = errors.New("repository credential activation pending")
var errCredentialActivationUnavailable = errors.New("repository credential activation unavailable")
var errCredentialActivationExhausted = errors.New("repository credential activation version exhausted")

type legacyCredentialView struct {
	phase      string
	epoch      uint64
	generation int64
	material   legacyRepositoryMaterial
	config     Settings
}

func (legacyCredentialView) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository credential view]")
}

type credentialActivationOptions struct {
	proxy         func(*http.Request) (*url.URL, error)
	persist       func(string, []byte) (bool, error)
	interrupt     func([]int64)
	beforePublish func() error
}

type repositoryCredentialActivation struct {
	store   *Store
	options credentialActivationOptions
	view    atomic.Pointer[legacyCredentialView]
}

func (a *repositoryCredentialActivation) active() bool {
	v := a.view.Load()
	return v != nil && v.phase == "active"
}

// Binding is startup-only. Main does not call this until all runtime consumers
// and recovery routes have been integrated; a partial gate must not be enabled.
func (s *SettingsService) bindRepositoryCredentialActivation(ctx context.Context, store *Store, options credentialActivationOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if store == nil || s.activation.Load() != nil {
		return errCredentialActivationUnavailable
	}
	if options.persist == nil {
		options.persist = func(path string, raw []byte) (bool, error) { return persistSettingsFile(path, raw, true) }
	}
	a := &repositoryCredentialActivation{store: store, options: options}
	a.view.Store(&legacyCredentialView{phase: "pending", epoch: 1})
	s.activation.Store(a)
	cfg := s.Snapshot()
	material, err := newLegacyRepositoryMaterial(cfg.GitLab.Token, cfg.GitLab.URL, options.proxy)
	if err != nil {
		return errCredentialActivationUnavailable
	}
	return a.activateAndPublish(ctx, s, cfg, material, 1, nil)
}

func (a *repositoryCredentialActivation) activateAndPublish(ctx context.Context, s *SettingsService, next Settings, material legacyRepositoryMaterial, epoch uint64, expected *legacyCredentialView) error {
	tx, err := a.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return errCredentialActivationPending
	}
	defer tx.Rollback()
	if expected != nil && validateLegacyRepositoryState(ctx, tx, expected.material, expected.generation) != nil {
		return errCredentialActivationPending
	}
	generation, changed, err := activateLegacyRepositoryStateTx(ctx, tx, material)
	if errors.Is(err, errLegacyGenerationExhausted) {
		return errCredentialActivationExhausted
	}
	if err != nil {
		return errCredentialActivationPending
	}
	var interrupted []int64
	if changed {
		interrupted, err = cancelLegacyCredentialRunsTx(ctx, tx)
		if err != nil {
			return errCredentialActivationPending
		}
	}
	if err = tx.Commit(); err != nil {
		return errCredentialActivationPending
	}
	if len(interrupted) > 0 && a.options.interrupt != nil {
		a.options.interrupt(interrupted)
	}
	if a.options.beforePublish != nil && a.options.beforePublish() != nil {
		return errCredentialActivationPending
	}
	next = cloneSettings(next)
	s.current.Store(&next)
	a.view.Store(&legacyCredentialView{phase: "active", epoch: epoch, generation: generation, material: material, config: next})
	return nil
}

// Settings.mu serializes writers. No reader holds that lock across network IO.
func (a *repositoryCredentialActivation) save(s *SettingsService, next Settings, raw []byte) error {
	old := a.view.Load()
	if old == nil || old.phase != "active" {
		return errCredentialActivationPending
	}
	material, err := newLegacyRepositoryMaterial(next.GitLab.Token, next.GitLab.URL, a.options.proxy)
	if err != nil {
		return errCredentialActivationUnavailable
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err = validateLegacyRepositoryState(ctx, a.store.DB, old.material, old.generation); err != nil {
		pending := *old
		pending.phase = "pending"
		a.view.Store(&pending)
		return errCredentialActivationUnavailable
	}
	changed := !hmac.Equal(old.material.encoded, material.encoded)
	if old.epoch == ^uint64(0) {
		return errCredentialActivationExhausted
	}
	epoch := old.epoch
	if changed {
		epoch++
		pending := *old
		pending.phase, pending.epoch = "pending", epoch
		a.view.Store(&pending)
	}
	replaced, err := a.options.persist(s.path, raw)
	if err != nil {
		if replaced {
			pending := *old
			pending.phase, pending.epoch = "pending", old.epoch+1
			a.view.Store(&pending)
			return errCredentialActivationPending
		}
		if changed {
			restored := *old
			restored.epoch = epoch
			a.view.Store(&restored)
		}
		return errCredentialActivationUnavailable
	}
	// Even an unchanged credential needs a gate during DB validation and full
	// config publication. Pending is also retained on commit uncertainty.
	pending := *old
	pending.phase, pending.epoch = "pending", epoch
	a.view.Store(&pending)
	return a.activateAndPublish(ctx, s, next, material, epoch, old)
}

type legacyCredentialDescriptor struct {
	activation *repositoryCredentialActivation
	view       *legacyCredentialView
}

func (legacyCredentialDescriptor) Format(s fmt.State, _ rune) {
	fmt.Fprint(s, "[private repository credential descriptor]")
}

func (s *SettingsService) captureLegacyCredential(ctx context.Context) (legacyCredentialDescriptor, error) {
	a := s.activation.Load()
	if a == nil {
		return legacyCredentialDescriptor{}, errCredentialActivationUnavailable
	}
	d := legacyCredentialDescriptor{activation: a, view: a.view.Load()}
	return d, d.validate(ctx)
}

func (d legacyCredentialDescriptor) validate(ctx context.Context) error {
	if d.activation == nil || d.view == nil || d.view.phase != "active" {
		return errCredentialActivationUnavailable
	}
	current := d.activation.view.Load()
	if current == nil || current.phase != "active" || current.epoch != d.view.epoch || current.generation != d.view.generation {
		return errCredentialActivationPending
	}
	if err := validateLegacyRepositoryState(ctx, d.activation.store.DB, current.material, d.view.generation); err != nil {
		return errCredentialActivationUnavailable
	}
	after := d.activation.view.Load()
	if after == nil || after.phase != "active" || after.epoch != d.view.epoch || after.generation != d.view.generation {
		return errCredentialActivationPending
	}
	return nil
}
