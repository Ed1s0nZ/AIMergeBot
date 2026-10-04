package platform

import (
	"context"
	"fmt"
	"path"
	"sort"

	"github.com/xanzy/go-gitlab"
)

// Only known-present entries are collected. Missing pages never establish absence.
func (g *GitLabRepository) metadataEntries(ctx context.Context, pid int, ref string, paths []string, requests *int) (map[string]*GitEntry, error) {
	groups := map[string]map[string]bool{}
	for _, p := range paths {
		if !validPath(p) {
			continue
		}
		dir := path.Dir(p)
		if dir == "." {
			dir = ""
		}
		if groups[dir] == nil {
			groups[dir] = map[string]bool{}
		}
		groups[dir][p] = true
	}
	dirs := []string{}
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	out := map[string]*GitEntry{}
	for _, dir := range dirs {
		wanted := groups[dir]
		for page := 1; page <= 20 && len(wanted) > 0 && *requests < 40; page++ {
			*requests++
			nodes, response, err := g.Client.Repositories.ListTree(pid, &gitlab.ListTreeOptions{Path: gitlab.String(dir), Ref: gitlab.String(ref), ListOptions: gitlab.ListOptions{Page: page, PerPage: 100}}, gitlab.WithContext(ctx))
			if err != nil {
				if ctx.Err() != nil || retryableError(err) {
					return nil, err
				}
				break // Caller records a metadata coverage gap for unresolved paths.
			}
			for _, node := range nodes {
				if node == nil || !wanted[node.Path] {
					continue
				}
				entry := &GitEntry{Mode: node.Mode, Type: node.Type, ObjectID: node.ID}
				if validGitEntry(entry) {
					out[node.Path] = entry
					delete(wanted, node.Path)
				}
			}
			if response.NextPage == 0 && len(nodes) < 100 {
				break
			}
		}
	}
	return out, nil
}
func (g *GitLabRepository) enrichMetadata(ctx context.Context, snap Snapshot, changes []Change) ([]string, error) {
	old, current := []string{}, []string{}
	for _, c := range changes {
		if !c.Added {
			old = append(old, c.OldPath)
		}
		if !c.Deleted {
			current = append(current, c.NewPath)
		}
	}
	requests := 0
	base, err := g.metadataEntries(ctx, snap.ProjectID, snap.BaseSHA, old, &requests)
	if err != nil {
		return nil, err
	}
	head, err := g.metadataEntries(ctx, snap.SourceProjectID, snap.HeadSHA, current, &requests)
	if err != nil {
		return nil, err
	}
	notes := []string{}
	for i := range changes {
		c := &changes[i]
		if !c.Added && base[c.OldPath] == nil || !c.Deleted && head[c.NewPath] == nil {
			note := fmt.Sprintf("Git metadata unavailable or lookup budget exceeded: %s", c.NewPath)
			notes = append(notes, note)
			c.Notes = append(c.Notes, note)
			continue
		}
		c.Metadata = changeMetadata(*c, base[c.OldPath], head[c.NewPath])
		if c.Metadata == nil {
			note := "Git metadata did not establish a change: " + c.NewPath
			notes = append(notes, note)
			c.Notes = append(c.Notes, note)
		}
	}
	return notes, nil
}
