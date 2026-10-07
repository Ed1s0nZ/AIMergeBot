package platform

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const githubDirectoryEntryLimit = 2000
const githubObjectEntryLimit = 20000
const githubFileReadLimit = 256 << 10
const githubPathByteLimit = 4096
const githubPathDepthLimit = 64

// All cached values are immutable and are scoped to a verified repository.
// The operation gate serializes API reads without holding a database transaction.
type githubObjectReader struct {
	client       *githubReadClient
	gate         chan struct{}
	roots        map[string]string
	trees        map[string][]githubTreeEntry
	entries      int
	listings     map[string][]string
	listingBytes int
}
type githubTreeEntry struct {
	Path      string
	Entry     GitEntry
	Size      int64
	SizeKnown bool
}

func newGitHubObjectReader(ctx context.Context, client *githubReadClient) (*githubObjectReader, error) {
	if client == nil {
		return nil, githubReadFailure("invalid_configuration")
	}
	if err := client.verifyRepository(ctx); err != nil {
		return nil, err
	}
	return &githubObjectReader{client: client, gate: make(chan struct{}, 1), roots: map[string]string{}, trees: map[string][]githubTreeEntry{}, listings: map[string][]string{}}, nil
}
func (g *githubObjectReader) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case g.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *githubObjectReader) release() { <-g.gate }
func validGitHubObjectSHA(sha string) bool {
	if len(sha) != 40 || strings.Trim(sha, "0") == "" {
		return false
	}
	for _, ch := range sha {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
func validGitHubObjectPath(value string, directory bool) bool {
	if value == "" {
		return directory
	}
	if len(value) > githubPathByteLimit || !utf8.ValidString(value) || !validPath(value) || len(strings.Split(value, "/")) > githubPathDepthLimit {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 {
			return false
		}
	}
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return false
		}
	}
	return true
}

// Helpers below require the operation gate. A response is cached only after all
// required identity/shape fields have been verified, never on partial success.
func (g *githubObjectReader) rootTree(ctx context.Context, commit string) (string, error) {
	if !validGitHubObjectSHA(commit) {
		return "", githubReadFailure("invalid_object_id")
	}
	if root, ok := g.roots[commit]; ok {
		return root, nil
	}
	var wire struct {
		SHA  string `json:"sha"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := g.client.get(ctx, "/git/commits/"+commit, nil, 256<<10, &wire); err != nil {
		return "", err
	}
	if wire.SHA != commit || !validGitHubObjectSHA(wire.Tree.SHA) {
		return "", githubReadFailure("object_identity_changed")
	}
	g.roots[commit] = wire.Tree.SHA
	return wire.Tree.SHA, nil
}
func (g *githubObjectReader) tree(ctx context.Context, sha string) ([]githubTreeEntry, error) {
	if !validGitHubObjectSHA(sha) {
		return nil, githubReadFailure("invalid_object_id")
	}
	if nodes, ok := g.trees[sha]; ok {
		return nodes, nil
	}
	var wire struct {
		SHA       string `json:"sha"`
		Truncated *bool  `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size *int64 `json:"size"`
		} `json:"tree"`
	}
	if err := g.client.get(ctx, "/git/trees/"+sha, nil, 2<<20, &wire); err != nil {
		return nil, err
	}
	if wire.SHA != sha || wire.Truncated == nil || wire.Tree == nil {
		return nil, githubReadFailure("invalid_tree_response")
	}
	if *wire.Truncated {
		return nil, githubReadFailure("tree_incomplete")
	}
	if len(wire.Tree) > githubDirectoryEntryLimit || g.entries+len(wire.Tree) > githubObjectEntryLimit {
		return nil, githubReadFailure("tree_budget_exceeded")
	}
	seen := map[string]bool{}
	nodes := make([]githubTreeEntry, 0, len(wire.Tree))
	for _, node := range wire.Tree {
		if !validGitHubObjectPath(node.Path, false) || strings.Contains(node.Path, "/") || seen[node.Path] || !validGitHubObjectSHA(node.SHA) {
			return nil, githubReadFailure("invalid_tree_response")
		}
		expected := entryType(node.Mode)
		if node.Mode == "040000" {
			expected = "tree"
		}
		if expected == "" || node.Type != expected || node.Size != nil && *node.Size < 0 || node.Type == "blob" && node.Size == nil {
			return nil, githubReadFailure("invalid_tree_response")
		}
		entry := githubTreeEntry{Path: node.Path, Entry: GitEntry{Mode: node.Mode, Type: node.Type, ObjectID: node.SHA}, SizeKnown: node.Size != nil}
		if node.Size != nil {
			entry.Size = *node.Size
		}
		seen[node.Path] = true
		nodes = append(nodes, entry)
	}
	g.entries += len(nodes)
	g.trees[sha] = nodes
	return nodes, nil
}
func (g *githubObjectReader) resolveDirectory(ctx context.Context, root, directory string) (string, error) {
	if directory == "" {
		return root, nil
	}
	current := root
	ancestors := map[string]bool{root: true}
	for _, segment := range strings.Split(directory, "/") {
		nodes, err := g.tree(ctx, current)
		if err != nil {
			return "", err
		}
		var found *githubTreeEntry
		for i := range nodes {
			if nodes[i].Path == segment {
				found = &nodes[i]
				break
			}
		}
		if found == nil {
			return "", githubReadFailure("path_absent")
		}
		if found.Entry.Mode != "040000" || found.Entry.Type != "tree" {
			return "", githubReadFailure("not_a_directory")
		}
		current = found.Entry.ObjectID
		if ancestors[current] {
			return "", githubReadFailure("invalid_tree_cycle")
		}
		ancestors[current] = true
	}
	return current, nil
}
func (g *githubObjectReader) directory(ctx context.Context, commit, directory string) ([]githubTreeEntry, error) {
	if !validGitHubObjectSHA(commit) || !validGitHubObjectPath(directory, true) {
		return nil, githubReadFailure("invalid_object_path")
	}
	if err := g.acquire(ctx); err != nil {
		return nil, err
	}
	defer g.release()
	root, err := g.rootTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	target, err := g.resolveDirectory(ctx, root, directory)
	if err != nil {
		return nil, err
	}
	nodes, err := g.tree(ctx, target)
	if err != nil {
		return nil, err
	}
	return append([]githubTreeEntry{}, nodes...), nil
}
func (g *githubObjectReader) readFileBytes(ctx context.Context, commit, file string) ([]byte, error) {
	if !validGitHubObjectSHA(commit) || !validGitHubObjectPath(file, false) {
		return nil, githubReadFailure("invalid_object_path")
	}
	if err := g.acquire(ctx); err != nil {
		return nil, err
	}
	defer g.release()
	root, err := g.rootTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(file, "/")
	target, err := g.resolveDirectory(ctx, root, strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return nil, err
	}
	nodes, err := g.tree(ctx, target)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if node.Path != parts[len(parts)-1] {
			continue
		}
		if node.Entry.Mode != "100644" && node.Entry.Mode != "100755" {
			return nil, githubReadFailure("not_a_regular_file")
		}
		return g.blob(ctx, node)
	}
	return nil, githubReadFailure("path_absent")
}
func (g *githubObjectReader) blob(ctx context.Context, node githubTreeEntry) ([]byte, error) {
	if !node.SizeKnown || node.Size < 0 || node.Size > githubFileReadLimit {
		return nil, githubReadFailure("file_read_budget_exceeded")
	}
	var wire struct {
		SHA      string  `json:"sha"`
		Encoding string  `json:"encoding"`
		Content  *string `json:"content"`
		Size     *int64  `json:"size"`
	}
	if err := g.client.get(ctx, "/git/blobs/"+node.Entry.ObjectID, nil, 512<<10, &wire); err != nil {
		return nil, err
	}
	if wire.SHA != node.Entry.ObjectID || wire.Encoding != "base64" || wire.Content == nil || wire.Size == nil || *wire.Size != node.Size {
		return nil, githubReadFailure("invalid_blob_response")
	}
	raw, err := base64.StdEncoding.DecodeString(*wire.Content)
	if err != nil || len(raw) > githubFileReadLimit || int64(len(raw)) != node.Size {
		return nil, githubReadFailure("invalid_blob_response")
	}
	hash := sha1.New()
	fmt.Fprintf(hash, "blob %d\x00", len(raw))
	hash.Write(raw)
	if hex.EncodeToString(hash.Sum(nil)) != node.Entry.ObjectID {
		return nil, githubReadFailure("blob_identity_changed")
	}
	return raw, nil
}
