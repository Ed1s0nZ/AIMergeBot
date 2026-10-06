package platform

import (
	"context"
	"errors"
	"fmt"
)

// Prepared sources are immutable for one audit, and never resolve moving refs.
type ContextSource struct {
	Repository Repository
	Snapshot   Snapshot
	Authorize  func(context.Context) error
}

type contextReadArgs struct {
	RepositoryID int    `json:"repository_id"`
	Path         string `json:"path"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
}
type contextSearchArgs struct {
	RepositoryID int    `json:"repository_id"`
	Query        string `json:"query"`
	Cursor       int    `json:"cursor"`
	Path         string `json:"path"`
	Extension    string `json:"extension"`
	Regex        bool   `json:"regex"`
	IgnoreCase   bool   `json:"ignore_case"`
}
type contextDirectoryArgs struct {
	RepositoryID int    `json:"repository_id"`
	Path         string `json:"path"`
	Depth        int    `json:"depth"`
	Cursor       int    `json:"cursor"`
}
type contextRepositoryStatus struct {
	ProjectID int    `json:"project_id"`
	SHA       string `json:"sha"`
	Available bool   `json:"available"`
}

func (t *auditTools) contextSource(id int) (ContextSource, error) {
	source, ok := t.contextSources[id]
	if !ok || id <= 0 {
		return ContextSource{}, ErrContextRepository
	}
	authorized := false
	for _, item := range contextPolicyItems(t.snap) {
		if item.ProjectID == id && item.SHA == source.Snapshot.HeadSHA && item.SHA == source.Snapshot.BaseSHA && source.Snapshot.ProjectID == id && source.Snapshot.SourceProjectID == id {
			authorized = true
		}
	}
	if !authorized {
		return ContextSource{}, ErrContextRepository
	}
	return source, nil
}
func (t *auditTools) contextReader(ctx context.Context, id int) (*auditTools, ContextSource, error) {
	source, err := t.contextSource(id)
	if err != nil {
		return nil, source, err
	}
	if source.Authorize != nil {
		if err = source.Authorize(ctx); err != nil {
			return nil, source, ErrContextRepository
		}
	}
	if source.Repository == nil {
		return nil, source, fmt.Errorf("fixed context repository unavailable")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.contextReaders == nil {
		t.contextReaders = map[int]*auditTools{}
	}
	reader := t.contextReaders[id]
	if reader == nil {
		reader = &auditTools{repo: source.Repository, snap: source.Snapshot, cache: map[string]string{}, rawOnly: true}
		t.contextReaders[id] = reader
	}
	return reader, source, nil
}
func (t *auditTools) invokeContext(ctx context.Context, name string, id int, args any, fn func(*auditTools) (toolOutput, error)) (toolOutput, error) {
	return t.invoke(name, args, func() (toolOutput, error) {
		reader, source, err := t.contextReader(ctx, id)
		if err != nil {
			return toolOutput{}, err
		}
		out, err := fn(reader)
		if err != nil {
			if errors.Is(err, ErrRepositoryUnavailable) {
				return toolOutput{RepositoryID: id}, fmt.Errorf("context repository read unavailable or incomplete: %w", ErrRepositoryUnavailable)
			}
			return toolOutput{RepositoryID: id}, fmt.Errorf("context repository read unavailable or incomplete")
		}
		// Recheck after network/object reads, including cached results.
		if source.Authorize != nil {
			if err = source.Authorize(ctx); err != nil {
				return toolOutput{RepositoryID: id}, ErrContextRepository
			}
		}
		out.RepositoryID = id
		return out, nil
	})
}
func (t *auditTools) contextRepositories(ctx context.Context, _ struct{}) (toolOutput, error) {
	return t.invoke("list_repositories", struct{}{}, func() (toolOutput, error) {
		out := toolOutput{Repositories: []contextRepositoryStatus{}}
		for _, item := range contextPolicyItems(t.snap) {
			source, err := t.contextSource(item.ProjectID)
			if err != nil {
				return toolOutput{}, err
			}
			if source.Authorize != nil {
				if err = source.Authorize(ctx); err != nil {
					return toolOutput{}, ErrContextRepository
				}
			}
			out.Repositories = append(out.Repositories, contextRepositoryStatus{item.ProjectID, item.SHA, source.Repository != nil})
		}
		// A later authorization callback may revoke an earlier item while
		// assembling the list. Do not expose a partially stale aggregate.
		for _, item := range contextPolicyItems(t.snap) {
			source, err := t.contextSource(item.ProjectID)
			if err != nil {
				return toolOutput{}, err
			}
			if source.Authorize != nil {
				if err := source.Authorize(ctx); err != nil {
					return toolOutput{}, ErrContextRepository
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return toolOutput{}, err
		}
		return out, nil
	})
}
func (t *auditTools) contextFile(ctx context.Context, a contextReadArgs) (toolOutput, error) {
	return t.invokeContext(ctx, "read_repository_file", a.RepositoryID, a, func(reader *auditTools) (toolOutput, error) {
		return reader.file(ctx, readArgs{Path: a.Path, Start: a.Start, End: a.End})
	})
}
func (t *auditTools) contextSearch(ctx context.Context, a contextSearchArgs) (toolOutput, error) {
	return t.invokeContext(ctx, "search_repository_code", a.RepositoryID, a, func(reader *auditTools) (toolOutput, error) {
		return reader.search(ctx, searchArgs{Query: a.Query, Cursor: a.Cursor, Path: a.Path, Extension: a.Extension, Regex: a.Regex, IgnoreCase: a.IgnoreCase})
	})
}
func (t *auditTools) contextDirectory(ctx context.Context, a contextDirectoryArgs) (toolOutput, error) {
	return t.invokeContext(ctx, "list_repository_directory", a.RepositoryID, a, func(reader *auditTools) (toolOutput, error) {
		return reader.directory(ctx, directoryArgs{Path: a.Path, Depth: a.Depth, Cursor: a.Cursor})
	})
}
