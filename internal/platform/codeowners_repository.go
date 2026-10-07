package platform

import (
	"context"
	"github.com/xanzy/go-gitlab"
	"strings"
)

func (g *GitLabRepository) CodeOwnersProvider() string { return "gitlab" }
func (g *GitLabRepository) CodeOwnerDirectory(ctx context.Context, s Snapshot, dir string) ([]CodeOwnerEntry, error) {
	if s.SourceProjectID <= 0 || !validCodeOwnerSHA(s.HeadSHA) || (dir != "" && dir != "docs" && dir != ".gitlab") {
		return nil, ErrCodeOwnersSource
	}
	out := []CodeOwnerEntry{}
	for page := 1; page <= 20; page++ {
		nodes, response, err := g.Client.Repositories.ListTree(s.SourceProjectID, &gitlab.ListTreeOptions{Path: gitlab.String(dir), Ref: gitlab.String(s.HeadSHA), Recursive: gitlab.Bool(false), ListOptions: gitlab.ListOptions{Page: page, PerPage: 100}}, gitlab.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		if response == nil || len(nodes) > 100 {
			return nil, ErrCodeOwnersSource
		}
		for _, node := range nodes {
			if node == nil || !validPath(node.Path) {
				return nil, ErrCodeOwnersSource
			}
			prefix := ""
			if dir != "" {
				prefix = dir + "/"
			}
			if !strings.HasPrefix(node.Path, prefix) || strings.Contains(strings.TrimPrefix(node.Path, prefix), "/") {
				return nil, ErrCodeOwnersSource
			}
			out = append(out, CodeOwnerEntry{Path: node.Path, Mode: node.Mode, Type: node.Type})
		}
		if response.NextPage == 0 && len(nodes) < 100 {
			return out, nil
		}
		if response.NextPage != 0 && response.NextPage != page+1 {
			return nil, ErrCodeOwnersSource
		}
	}
	return nil, ErrCodeOwnersSource
}
func (d *DynamicRepository) CodeOwnersProvider() string { return "gitlab" }
func (d *DynamicRepository) CodeOwnerDirectory(ctx context.Context, s Snapshot, dir string) ([]CodeOwnerEntry, error) {
	r, err := d.repo()
	if err != nil {
		return nil, err
	}
	return r.CodeOwnerDirectory(ctx, s, dir)
}
func (g *GitRepository) CodeOwnersProvider() string {
	if source, ok := g.Metadata.(CodeOwnerDirectoryReader); ok {
		return source.CodeOwnersProvider()
	}
	return ""
}
func (g *GitRepository) CodeOwnerDirectory(ctx context.Context, s Snapshot, dir string) ([]CodeOwnerEntry, error) {
	ref, err := g.ref(s, false)
	if err != nil || (dir != "" && dir != "docs" && dir != ".gitlab" && dir != ".github") {
		return nil, ErrCodeOwnersSource
	}
	object := ref
	if dir != "" {
		object += ":" + dir
	}
	raw, err := g.command(ctx, "ls-tree", "-z", object)
	if err != nil {
		return nil, err
	}
	out := []CodeOwnerEntry{}
	for _, row := range strings.Split(raw, "\x00") {
		if row == "" {
			continue
		}
		parts := strings.SplitN(row, "\t", 2)
		if len(parts) != 2 || !validPath(parts[1]) || strings.Contains(parts[1], "/") {
			return nil, ErrCodeOwnersSource
		}
		metadata := strings.Fields(parts[0])
		if len(metadata) != 3 {
			return nil, ErrCodeOwnersSource
		}
		path := parts[1]
		if dir != "" {
			path = dir + "/" + path
		}
		out = append(out, CodeOwnerEntry{Path: path, Mode: metadata[0], Type: metadata[1]})
		if len(out) > 2000 {
			return nil, ErrCodeOwnersSource
		}
	}
	return out, nil
}
