package platform

import "context"

type repositoryCredentialsContextKey struct{}

func capturedRepositoryCredentials(ctx context.Context) []legacyCredentialDescriptor {
	items, _ := ctx.Value(repositoryCredentialsContextKey{}).([]legacyCredentialDescriptor)
	return items
}

func withCapturedRepositoryCredential(ctx context.Context, descriptor legacyCredentialDescriptor) context.Context {
	items := capturedRepositoryCredentials(ctx)
	for _, item := range items {
		if item.activation == descriptor.activation && item.view != nil && descriptor.view != nil && item.view.epoch == descriptor.view.epoch && item.view.generation == descriptor.view.generation {
			return ctx
		}
	}
	items = append(append([]legacyCredentialDescriptor{}, items...), descriptor)
	return context.WithValue(ctx, repositoryCredentialsContextKey{}, items)
}

// Background persistence retains the captured identity, not callbacks that
// would reacquire Store.DB while the caller already holds its only connection.
func copyRepositoryCredentials(dst, source context.Context) context.Context {
	for _, descriptor := range capturedRepositoryCredentials(source) {
		dst = withCapturedRepositoryCredential(dst, descriptor)
	}
	return dst
}

func scopeUsesLegacyCredential(scope *RepositoryScope) bool {
	if scope == nil {
		return true
	}
	if scope.Target.Credential.Kind == "legacy_gitlab" || scope.Source.Credential.Kind == "legacy_gitlab" {
		return true
	}
	for _, item := range scope.Context {
		if item.Repository.Credential.Kind == "legacy_gitlab" {
			return true
		}
	}
	return false
}

func validateRepositoryCredentialsTx(ctx context.Context, s *Store, q queryRower, snap Snapshot) error {
	items := capturedRepositoryCredentials(ctx)
	for _, descriptor := range items {
		if descriptor.activation == nil || descriptor.activation.store != s || descriptor.validateWithQuery(ctx, q) != nil {
			return ErrRepositoryUnavailable
		}
	}
	// Explicit native-only scopes do not depend on the global legacy credential.
	if snap.AuditPolicy != nil && !scopeUsesLegacyCredential(snap.AuditPolicy.RepositoryScope) {
		return nil
	}
	if activation := s.repositoryCredentials.Load(); activation != nil {
		descriptor := legacyCredentialDescriptor{activation: activation, view: activation.view.Load()}
		if activation.store != s || descriptor.validateWithQuery(ctx, q) != nil {
			return ErrRepositoryUnavailable
		}
	}
	return nil
}

func validateRunRepositoryCredentialsTx(ctx context.Context, s *Store, q queryRower, id int64) error {
	bound := s.repositoryCredentials.Load() != nil
	if !bound && len(capturedRepositoryCredentials(ctx)) == 0 {
		return nil
	}
	snap, err := snapshotForRun(ctx, q, id)
	if err != nil {
		return ErrRepositoryUnavailable
	}
	return validateRepositoryCredentialsTx(ctx, s, q, snap)
}
