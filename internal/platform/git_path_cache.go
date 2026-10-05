package platform

import (
	"context"
	"fmt"
	"strings"
)

const gitPathCacheLimit = 8 * 1024 * 1024

type gitPathCacheEntry struct {
	directory string
	sha       string
	paths     []string
	bytes     int
}

func (g *GitRepository) cachedPaths(ctx context.Context, sha string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.pathMu.Lock()
	defer g.pathMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, entry := range g.pathCache {
		if entry.directory == g.Directory && entry.sha == sha {
			return append([]string{}, entry.paths...), nil
		}
	}
	raw, err := g.command(ctx, "ls-tree", "-r", "-z", "--name-only", sha)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	if raw != "" {
		paths = strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	}
	// Account for the retained raw string, string headers and entry overhead.
	size := len(raw) + len(paths)*16 + len(g.Directory) + len(sha) + 128
	if size > gitPathCacheLimit {
		return nil, fmt.Errorf("repository file listing exceeds 8 MiB cache budget")
	}
	for len(g.pathCache) > 0 && (len(g.pathCache) >= 2 || g.pathCacheBytes+size > gitPathCacheLimit) {
		g.pathCacheBytes -= g.pathCache[0].bytes
		g.pathCache[0] = gitPathCacheEntry{}
		g.pathCache = g.pathCache[1:]
	}
	g.pathCache = append(g.pathCache, gitPathCacheEntry{directory: g.Directory, sha: sha, paths: paths, bytes: size})
	g.pathCacheBytes += size
	return append([]string{}, paths...), nil
}
