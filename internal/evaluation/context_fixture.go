package evaluation

import (
	"context"
	"fmt"
	"path/filepath"
	"pr_agent/internal/platform"
)

type ContextFixture struct {
	RepositoryID int               `json:"repository_id"`
	Files        map[string]string `json:"files"`
}

func validateContextFixtures(c Case) error {
	if len(c.ContextRepositories) > 8 || (c.Git != nil && len(c.ContextRepositories) > 0) {
		return fmt.Errorf("unsupported context fixture scope")
	}
	seen := map[int]bool{}
	for _, source := range c.ContextRepositories {
		if source.RepositoryID < 2 || seen[source.RepositoryID] || len(source.Files) == 0 || len(source.Files) > 100 {
			return fmt.Errorf("invalid context fixture ID or file count")
		}
		seen[source.RepositoryID] = true
		for path, text := range source.Files {
			if !safeFixturePath(path) || len(text) > 256*1024 {
				return fmt.Errorf("unsafe context fixture or byte budget")
			}
		}
	}
	return nil
}

// Only caller-selected synthetic fixtures; no network, code execution or implicit discovery.
func PrepareContextFixtures(ctx context.Context, root string, c Case) (map[int]platform.ContextSource, []platform.ContextRepository, error) {
	if err := validateContextFixtures(c); err != nil {
		return nil, nil, err
	}
	sources := map[int]platform.ContextSource{}
	policy := []platform.ContextRepository{}
	for _, source := range c.ContextRepositories {
		dir := filepath.Join(root, fmt.Sprintf("context-%d", source.RepositoryID))
		_, sha, err := BuildRepository(ctx, dir, Case{BaseFiles: source.Files, HeadFiles: source.Files})
		if err != nil {
			return nil, nil, err
		}
		sources[source.RepositoryID] = platform.ContextSource{Repository: &platform.GitRepository{Directory: dir}, Snapshot: platform.Snapshot{ProjectID: source.RepositoryID, SourceProjectID: source.RepositoryID, BaseSHA: sha, HeadSHA: sha}, Authorize: func(ctx context.Context) error { return ctx.Err() }}
		policy = append(policy, platform.ContextRepository{ProjectID: source.RepositoryID, SHA: sha})
	}
	return sources, policy, nil
}
