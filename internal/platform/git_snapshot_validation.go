package platform

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ValidatePinnedCommits rejects moving refs and non-commit object IDs before
// a historical PR is sent to the model. Object reads are bounded and read-only.
func (g *GitRepository) ValidatePinnedCommits(ctx context.Context, base, head string) error {
	// Require the actual repository root, so an artifact beside a supplied
	// subdirectory cannot accidentally become ground truth inside the repository.
	bare, err := g.command(ctx, "rev-parse", "--is-bare-repository")
	if err != nil {
		return err
	}
	root := g.Directory
	if strings.TrimSpace(bare) != "true" {
		root, err = g.command(ctx, "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
	}
	actual, err := filepath.EvalSymlinks(strings.TrimSpace(root))
	if err != nil {
		return err
	}
	given, err := filepath.EvalSymlinks(g.Directory)
	if err != nil {
		return err
	}
	if actual != given {
		return fmt.Errorf("historical snapshot requires repository root directory")
	}
	for _, sha := range []string{base, head} {
		if !commitID.MatchString(sha) {
			return fmt.Errorf("full commit IDs required")
		}
		kind, err := g.command(ctx, "cat-file", "-t", sha)
		if err != nil {
			return err
		}
		if strings.TrimSpace(kind) != "commit" {
			return fmt.Errorf("snapshot object is not a commit")
		}
	}
	return nil
}
