package evaluation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"pr_agent/internal/platform"
	"regexp"
	"strings"
)

func validatePinnedCase(c PinnedGitCase) error {
	sha := regexp.MustCompile(`^[a-fA-F0-9]{40}([a-fA-F0-9]{24})?$`)
	if !filepath.IsAbs(c.Directory) || filepath.Clean(c.Directory) != c.Directory || strings.ContainsRune(c.Directory, 0) || !sha.MatchString(c.BaseSHA) || !sha.MatchString(c.HeadSHA) {
		return fmt.Errorf("historical PR requires absolute local directory and full commit IDs")
	}
	return nil
}

// PrepareCase never checks out, executes or mutates a supplied repository.
func PrepareCase(ctx context.Context, fixtureDir string, c Case) (*platform.GitRepository, string, string, error) {
	if c.Git != nil {
		if err := validatePinnedCase(*c.Git); err != nil {
			return nil, "", "", err
		}
		repo := &platform.GitRepository{Directory: c.Git.Directory}
		if err := repo.ValidatePinnedCommits(ctx, c.Git.BaseSHA, c.Git.HeadSHA); err != nil {
			return nil, "", "", fmt.Errorf("historical PR commit objects unavailable")
		}
		return repo, c.Git.BaseSHA, c.Git.HeadSHA, nil
	}
	base, head, err := BuildRepository(ctx, fixtureDir, c)
	if err != nil {
		return nil, "", "", err
	}
	return &platform.GitRepository{Directory: fixtureDir}, base, head, nil
}

func resolvedLocation(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	tail := []string{}
	for {
		if _, err = os.Lstat(current); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot resolve location")
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		current = filepath.Join(current, tail[i])
	}
	return current, nil
}

// Ground truth and receipts must not enter any audited repository, including
// through symlinked parents. Paths are private and never printed in errors.
func ValidateEvaluationLocation(c Corpus, path string) error {
	target, err := resolvedLocation(path)
	if err != nil {
		return fmt.Errorf("cannot resolve evaluation artifact location")
	}
	for _, item := range c.Cases {
		if item.Git == nil {
			continue
		}
		root, err := resolvedLocation(item.Git.Directory)
		if err != nil {
			return fmt.Errorf("cannot resolve historical repository")
		}
		rel, err := filepath.Rel(root, target)
		if err != nil {
			return fmt.Errorf("cannot compare artifact location")
		}
		if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("evaluation artifacts must remain outside audited repositories")
		}
	}
	return nil
}
