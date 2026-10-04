package platform

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type GitEntry struct {
	Mode     string `json:"mode"`
	Type     string `json:"type"`
	ObjectID string `json:"object_id"`
}
type GitChangeMetadata struct {
	Kind    string    `json:"kind"`
	OldPath string    `json:"old_path"`
	NewPath string    `json:"new_path"`
	Base    *GitEntry `json:"base"`
	Head    *GitEntry `json:"head"`
}

func entryType(mode string) string {
	switch mode {
	case "100644", "100755", "120000":
		return "blob"
	case "160000":
		return "commit"
	}
	return ""
}
func validGitEntry(entry *GitEntry) bool {
	return entry == nil || entryType(entry.Mode) != "" && entry.Type == entryType(entry.Mode) && commitID.MatchString(entry.ObjectID) && strings.Trim(entry.ObjectID, "0") != ""
}
func (m GitChangeMetadata) canonical() string { raw, _ := json.Marshal(m); return string(raw) }
func (m GitChangeMetadata) valid() bool {
	if !validPath(m.OldPath) || !validPath(m.NewPath) || !validGitEntry(m.Base) || !validGitEntry(m.Head) || m.Base == nil && m.Head == nil {
		return false
	}
	switch m.Kind {
	case "add":
		return m.Base == nil && m.Head != nil
	case "delete":
		return m.Base != nil && m.Head == nil
	case "rename", "copy":
		return m.Base != nil && m.Head != nil && m.OldPath != m.NewPath
	case "modify", "type_change":
		return m.Base != nil && m.Head != nil && *m.Base != *m.Head
	}
	return false
}
func changeMetadata(c Change, base, head *GitEntry) *GitChangeMetadata {
	kind := "modify"
	if c.Added {
		kind = "add"
	} else if c.Deleted {
		kind = "delete"
	} else if c.Renamed {
		kind = "rename"
	} else if base != nil && head != nil && (base.Type != head.Type || (base.Mode == "120000") != (head.Mode == "120000")) {
		kind = "type_change"
	}
	m := &GitChangeMetadata{Kind: kind, OldPath: c.OldPath, NewPath: c.NewPath, Base: base, Head: head}
	if !m.valid() {
		return nil
	}
	return m
}

// No textual hunk is expected for unchanged blobs (e.g. chmod/rename) or gitlinks.
// Gitlink metadata describes only the recorded reference, never the external repository.
func (m GitChangeMetadata) metadataOnly() bool {
	if m.Base != nil && m.Head != nil && m.Base.ObjectID == m.Head.ObjectID {
		return true
	}
	return (m.Base == nil || m.Base.Type == "commit") && (m.Head == nil || m.Head.Type == "commit")
}
func (d DiffScope) metadataChanges() []GitChangeMetadata {
	keys := make([]string, 0, len(d.Metadata))
	for key := range d.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]GitChangeMetadata, 0, len(keys))
	for _, key := range keys {
		out = append(out, d.Metadata[key])
	}
	return out
}
func validateMetadataFinding(scope DiffScope, f *Finding) error {
	if f.Line != 0 {
		return fmt.Errorf("Git metadata anchor must not invent a line number")
	}
	for _, metadata := range scope.Metadata {
		p, entry := metadata.NewPath, metadata.Head
		if f.Side == "base" {
			p, entry = metadata.OldPath, metadata.Base
		}
		if entry == nil || p != f.File || f.Evidence != metadata.canonical() {
			continue
		}
		if f.Metadata != nil && f.Metadata.canonical() != metadata.canonical() {
			return fmt.Errorf("metadata attachment does not match pinned change")
		}
		copy := metadata
		f.Metadata = &copy
		return nil
	}
	return fmt.Errorf("metadata finding does not match an included pinned change")
}

func parseRawGitChanges(raw string) ([]Change, error) {
	if raw == "" {
		return []Change{}, nil
	}
	if !strings.HasSuffix(raw, "\x00") {
		return nil, fmt.Errorf("truncated Git raw diff")
	}
	parts := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	out := []Change{}
	for i := 0; i < len(parts); {
		fields := strings.Fields(parts[i])
		i++
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || i >= len(parts) {
			return nil, fmt.Errorf("invalid Git raw diff")
		}
		status := fields[4]
		if status == "" {
			return nil, fmt.Errorf("empty Git change status")
		}
		old := parts[i]
		i++
		c := Change{OldPath: old, NewPath: old, Added: status[0] == 'A', Deleted: status[0] == 'D', Renamed: status[0] == 'R' || status[0] == 'C'}
		if c.Renamed {
			if i >= len(parts) {
				return nil, fmt.Errorf("truncated Git rename")
			}
			c.NewPath = parts[i]
			i++
		}
		entry := func(mode, object string) (*GitEntry, error) {
			if mode == "000000" && strings.Trim(object, "0") == "" && (len(object) == 40 || len(object) == 64) {
				return nil, nil
			}
			item := &GitEntry{Mode: mode, ObjectID: object, Type: entryType(mode)}
			if !validGitEntry(item) {
				return nil, fmt.Errorf("invalid Git mode/object")
			}
			return item, nil
		}
		base, err := entry(strings.TrimPrefix(fields[0], ":"), fields[2])
		if err != nil {
			return nil, err
		}
		head, err := entry(fields[1], fields[3])
		if err != nil {
			return nil, err
		}
		c.Metadata = changeMetadata(c, base, head)
		if status[0] == 'C' && c.Metadata != nil {
			c.Metadata.Kind = "copy"
		}
		if c.Metadata == nil {
			return nil, fmt.Errorf("invalid or unchanged Git metadata record")
		}
		out = append(out, c)
	}
	return out, nil
}
