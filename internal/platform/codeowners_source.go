package platform

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrCodeOwnersSource = errors.New("CODEOWNERS source unavailable or lookup budget exceeded")

type CodeOwnerEntry struct{ Path, Mode, Type string }

// CodeOwnerDirectoryReader returns a complete directory listing, or an error.
// A partial listing must never certify that a higher-priority file is absent.
type CodeOwnerDirectoryReader interface {
	CodeOwnersProvider() string
	CodeOwnerDirectory(context.Context, Snapshot, string) ([]CodeOwnerEntry, error)
}
type CodeOwnerDocument struct {
	Provider     string          `json:"provider"`
	RepositoryID int             `json:"repository_id"`
	SHA          string          `json:"sha"`
	Path         string          `json:"path"`
	Present      bool            `json:"present"`
	Rules        *CodeOwnerRules `json:"-"`
}

func LoadCodeOwnerDocument(ctx context.Context, repo Repository, snap Snapshot) (CodeOwnerDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	doc := CodeOwnerDocument{RepositoryID: snap.SourceProjectID, SHA: snap.HeadSHA}
	if dynamic, ok := repo.(*DynamicRepository); ok {
		frozen, err := dynamic.repo()
		if err != nil {
			return doc, err
		}
		repo = frozen
	}
	source, ok := repo.(CodeOwnerDirectoryReader)
	if !ok || snap.SourceProjectID <= 0 || !validCodeOwnerSHA(snap.HeadSHA) {
		return doc, ErrCodeOwnersSource
	}
	if validator, ok := repo.(interface{ validateCodeOwnerSnapshot(Snapshot) error }); ok {
		if err := validator.validateCodeOwnerSnapshot(snap); err != nil {
			return doc, err
		}
	}
	doc.Provider = source.CodeOwnersProvider()
	var locations []string
	switch doc.Provider {
	case "github":
		locations = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}
	case "gitlab":
		locations = []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitlab/CODEOWNERS"}
	default:
		return doc, ErrCodeOwnersSource
	}
	root, err := source.CodeOwnerDirectory(ctx, snap, "")
	if err != nil {
		return doc, err
	}
	cache := map[string][]CodeOwnerEntry{"": root}
	for _, location := range locations {
		dir := ""
		if index := strings.LastIndexByte(location, '/'); index >= 0 {
			dir = location[:index]
		}
		entries, loaded := cache[dir]
		if !loaded {
			found := false
			for _, entry := range root {
				if entry.Path == dir {
					if entry.Type != "tree" || entry.Mode != "040000" {
						return doc, ErrCodeOwnersSource
					}
					found = true
					break
				}
			}
			if !found {
				continue
			}
			entries, err = source.CodeOwnerDirectory(ctx, snap, dir)
			if err != nil {
				return doc, err
			}
			cache[dir] = entries
		}
		for _, entry := range entries {
			if entry.Path != location {
				continue
			}
			doc.Present = true
			doc.Path = location
			if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
				return doc, ErrCodeOwnersSource
			}
			raw, err := repo.ReadFile(ctx, snap, location, false)
			if err != nil {
				return doc, err
			}
			doc.Rules, err = ParseCodeOwnerRules(doc.Provider, raw)
			return doc, err
		}
	}
	return doc, nil
}

func validCodeOwnerSHA(sha string) bool {
	return commitID.MatchString(sha) && strings.Trim(sha, "0") != ""
}
