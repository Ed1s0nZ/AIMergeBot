package platform

import (
	"context"
	"sort"
)

const githubListingPathByteLimit = 8 << 20

type githubTreeWalk struct {
	sha, prefix string
	ancestors   []string
}

// A list is published only when every directory has been read completely.
// Cached shared trees may appear at distinct paths, so work counts expanded
// entries as well as HTTP/cache entries to avoid exponential DAG expansion.
func (g *githubObjectReader) listFiles(ctx context.Context, commit string, page int) ([]string, bool, error) {
	if !validGitHubObjectSHA(commit) || page < 1 || page > 20 {
		return nil, false, githubReadFailure("invalid_listing_request")
	}
	if err := g.acquire(ctx); err != nil {
		return nil, false, err
	}
	defer g.release()
	root, err := g.rootTree(ctx, commit)
	if err != nil {
		return nil, false, err
	}
	paths, ok := g.listings[root]
	if !ok {
		paths, err = g.completePaths(ctx, root)
		if err != nil {
			return nil, false, err
		}
		size := 0
		for _, p := range paths {
			size += len(p)
		}
		if g.listingBytes+size > githubListingPathByteLimit {
			return nil, false, githubReadFailure("file_listing_budget_exceeded")
		}
		g.listingBytes += size
		g.listings[root] = paths
	}
	start := (page - 1) * 100
	if start >= len(paths) {
		return []string{}, false, nil
	}
	end := start + 100
	if end > len(paths) {
		end = len(paths)
	}
	return append([]string{}, paths[start:end]...), end < len(paths), nil
}
func (g *githubObjectReader) completePaths(ctx context.Context, root string) ([]string, error) {
	queue := []githubTreeWalk{{sha: root, ancestors: []string{root}}}
	paths := []string{}
	expanded, pathBytes := 0, 0
	for cursor := 0; cursor < len(queue); cursor++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current := queue[cursor]
		nodes, err := g.tree(ctx, current.sha)
		if err != nil {
			return nil, err
		}
		expanded += len(nodes)
		if expanded > githubObjectEntryLimit {
			return nil, githubReadFailure("file_listing_budget_exceeded")
		}
		for _, node := range nodes {
			p := node.Path
			if current.prefix != "" {
				p = current.prefix + "/" + p
			}
			if !validGitHubObjectPath(p, false) {
				return nil, githubReadFailure("file_listing_budget_exceeded")
			}
			pathBytes += len(p)
			if pathBytes > githubListingPathByteLimit {
				return nil, githubReadFailure("file_listing_budget_exceeded")
			}
			if node.Entry.Mode == "040000" {
				for _, parent := range current.ancestors {
					if parent == node.Entry.ObjectID {
						return nil, githubReadFailure("invalid_tree_cycle")
					}
				}
				ancestors := append(append([]string{}, current.ancestors...), node.Entry.ObjectID)
				queue = append(queue, githubTreeWalk{sha: node.Entry.ObjectID, prefix: p, ancestors: ancestors})
			} else if node.Entry.Mode == "100644" || node.Entry.Mode == "100755" {
				paths = append(paths, p)
				if len(paths) > 2000 {
					return nil, githubReadFailure("file_listing_budget_exceeded")
				}
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}
