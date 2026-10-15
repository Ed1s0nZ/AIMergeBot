package platform

import (
	"context"
	"io"
)

type repositoryFenceContextKey struct{}
type repositoryOperationFence func(context.Context) error

// Child repository contexts may add restrictions, never replace the parent's
// credential/ACL fence with a newly captured credential.
func withRepositoryFence(ctx context.Context, next repositoryOperationFence) context.Context {
	if next == nil {
		return ctx
	}
	if previous, ok := ctx.Value(repositoryFenceContextKey{}).(repositoryOperationFence); ok {
		child := next
		next = func(check context.Context) error {
			if err := previous(check); err != nil {
				return err
			}
			return child(check)
		}
	}
	return context.WithValue(ctx, repositoryFenceContextKey{}, next)
}

func checkRepositoryFence(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if check, ok := ctx.Value(repositoryFenceContextKey{}).(repositoryOperationFence); ok {
		if err := check(ctx); err != nil {
			return ErrRepositoryUnavailable
		}
	}
	return nil
}

type repositoryFencedBody struct {
	io.ReadCloser
	ctx context.Context
}

func (b *repositoryFencedBody) Read(p []byte) (int, error) {
	if err := checkRepositoryFence(b.ctx); err != nil {
		b.ReadCloser.Close()
		return 0, err
	}
	n, readErr := b.ReadCloser.Read(p)
	if err := checkRepositoryFence(b.ctx); err != nil {
		clear(p[:n])
		b.ReadCloser.Close()
		return 0, err
	}
	return n, readErr
}

// The descriptor and config are observed together. This remains compatible
// while production startup has not yet bound the activation protocol.
func (s *SettingsService) legacyOperationContext(ctx context.Context) (context.Context, Settings, error) {
	if s.activation.Load() == nil {
		return ctx, s.Snapshot(), checkRepositoryFence(ctx)
	}
	descriptor, err := s.captureLegacyCredential(ctx)
	if err != nil {
		return ctx, Settings{}, ErrRepositoryUnavailable
	}
	ctx = withRepositoryFence(ctx, descriptor.validate)
	ctx = withCapturedRepositoryCredential(ctx, descriptor)
	return ctx, cloneSettings(descriptor.view.config), checkRepositoryFence(ctx)
}
