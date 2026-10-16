package platform

import (
	"context"
	"encoding/json"
)

// Local authorization of a frozen reference; no remote observation is inferred.
func (s *Store) authorizeRepositoryReference(ctx context.Context, q queryRower, r RepositoryReference) error {
	if r.validateVersion(2) != nil {
		return ErrRepositoryUnavailable
	}
	identity, err := readRepositoryIdentity(ctx, q, r.ProjectID)
	if err != nil || identity.Health != "valid" || identity.Revision != r.IdentityRevision {
		return ErrRepositoryUnavailable
	}
	var enabled bool
	if err = q.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, r.ProjectID).Scan(&enabled); err != nil || !enabled {
		return ErrRepositoryUnavailable
	}
	if r.Credential.Kind == "legacy_gitlab" {
		if identity.Kind != "legacy" {
			return ErrRepositoryUnavailable
		}
		binding, err := readRepositoryBinding(ctx, q, r.ProjectID)
		if err != nil || binding.Revision != 0 {
			return ErrRepositoryUnavailable
		}
		a := s.repositoryCredentials.Load()
		if a == nil || a.store != s {
			return ErrRepositoryUnavailable
		}
		d := legacyCredentialDescriptor{activation: a, view: a.view.Load()}
		if d.validateWithQuery(ctx, q) != nil || d.view.generation != r.Credential.LegacyRepositoryRevision || d.view.config.GitLab.URL != r.LegacyEndpoint.ConfigURL {
			return ErrRepositoryUnavailable
		}
		var profile struct {
			Version int    `json:"version"`
			Policy  string `json:"transport_policy"`
		}
		if json.Unmarshal(d.view.material.encoded, &profile) != nil || profile.Version != 2 || profile.Policy != legacyFixedSessionPolicy {
			return ErrRepositoryUnavailable
		}
		return nil
	}
	if identity.Kind != "internal" {
		return ErrRepositoryUnavailable
	}
	b, err := readRepositoryBinding(ctx, q, r.ProjectID)
	if err != nil || b.Revision != r.BindingRevision || b.Provider != r.Provider || b.APIOrigin != r.APIOrigin || b.RemoteID != r.RemoteID || b.FullName != r.FullName || b.IntegrationID != r.Credential.IntegrationID {
		return ErrRepositoryUnavailable
	}
	integration, credentials, err := scanIntegration(q.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=? AND length(CAST(credentials AS BLOB))<=65536 AND length(CAST(project_ids AS BLOB))<=65536`, b.IntegrationID))
	if err != nil || !integration.Enabled || integration.Kind != r.Provider || integration.Revision != r.Credential.IntegrationRevision || validateIntegrationCredentials(r.Provider, credentials, true) != nil {
		return ErrRepositoryUnavailable
	}
	origin, err := canonicalRepositoryAPIOrigin(r.Provider, credentials.Endpoint)
	if err != nil || origin != r.APIOrigin {
		return ErrRepositoryUnavailable
	}
	for _, id := range integration.ProjectIDs {
		if id == r.ProjectID {
			return nil
		}
	}
	return ErrRepositoryUnavailable
}

func (s *Store) authorizeRepositoryBudgetSnapshot(ctx context.Context, q queryRower, snap Snapshot, actor int64) error {
	p := snap.AuditPolicy
	if actor <= 0 || p == nil || p.RepositoryScope == nil || p.RepositoryScope.Version != 2 || p.RepositoryBudget == nil || p.RepositoryBudget.validateFor(p, "audit") != nil || validateRepositoryScopeSnapshot(snap) != nil {
		return ErrRepositoryUnavailable
	}
	if _, err := requireSnapshotRole(ctx, q, snap, actor, "operator"); err != nil {
		return ErrRepositoryUnavailable
	}
	if err := validateRepositoryCredentialsTx(ctx, s, q, snap); err != nil {
		return ErrRepositoryUnavailable
	}
	refs := []RepositoryReference{p.RepositoryScope.Target, p.RepositoryScope.Source}
	for _, c := range p.RepositoryScope.Context {
		refs = append(refs, c.Repository)
	}
	seen := map[int]bool{}
	for _, r := range refs {
		if seen[r.ProjectID] {
			continue
		}
		seen[r.ProjectID] = true
		if s.authorizeRepositoryReference(ctx, q, r) != nil {
			return ErrRepositoryUnavailable
		}
	}
	return nil
}
