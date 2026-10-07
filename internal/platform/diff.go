package platform

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var hunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

type DiffScope struct {
	Included   []string
	Metadata   map[string]GitChangeMetadata
	Excluded   []string
	Removed    map[string]map[int]bool
	Text       string
	Added      map[string]map[int]bool
	Formatting map[string]FileFormattingScope
	Notes      []string
}

func BuildDiff(changes []Change, excluded []string, maxBytes int) DiffScope {
	d := DiffScope{Metadata: map[string]GitChangeMetadata{}, Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Formatting: map[string]FileFormattingScope{}, Notes: []string{}}
	var out strings.Builder
	for _, c := range changes {
		p := c.NewPath
		if c.Deleted {
			p = c.OldPath
		}
		skip := false
		for _, ext := range excluded {
			if strings.EqualFold(strings.TrimPrefix(path.Ext(p), "."), strings.TrimPrefix(ext, ".")) {
				skip = true
				break
			}
		}
		if skip {
			d.Excluded = append(d.Excluded, p)
			continue
		}
		if !validPath(p) {
			d.Notes = append(d.Notes, "Invalid path omitted")
			continue
		}
		d.Notes = append(d.Notes, c.Notes...)
		metadata := ""
		if c.Metadata != nil && c.Metadata.valid() {
			metadata = "Git metadata: " + c.Metadata.canonical() + "\n"
		}
		noText := c.Diff == "" || strings.HasPrefix(c.Diff, "Binary files")
		metadataOnly := c.Metadata != nil && c.Metadata.valid() && c.Metadata.metadataOnly()
		if noText && !metadataOnly {
			d.Notes = append(d.Notes, "No textual diff available: "+p)
		}
		if noText && metadata == "" {
			continue
		}
		textual := c.Diff
		if noText || metadataOnly {
			textual = ""
		}
		section := fmt.Sprintf("File: %s (old: %s, deleted: %t, renamed: %t)\n%s%s\n", p, c.OldPath, c.Deleted, c.Renamed, metadata, textual)
		if out.Len()+len(section) > maxBytes {
			d.Notes = append(d.Notes, "Diff budget exceeded; omitted: "+p)
			continue
		}
		out.WriteString(section)
		d.Included = append(d.Included, p)
		if metadata != "" {
			d.Metadata[p] = *c.Metadata
		}
		if textual == "" {
			continue
		}
		if stat, ok := formattingScopeForDiff(textual); ok {
			stat.Path = p
			merged := d.Formatting[p]
			merged.mergeFile(stat)
			merged.Path = p
			d.Formatting[p] = merged
		}
		lines := map[int]bool{}
		removed := map[int]bool{}
		oldPath := c.OldPath
		if oldPath == "" {
			oldPath = p
		}
		oldLine := 0
		n := 0
		inHunk := false
		for _, line := range strings.Split(textual, "\n") {
			if m := hunkPattern.FindStringSubmatch(line); m != nil {
				n, _ = strconv.Atoi(m[2])
				oldLine, _ = strconv.Atoi(m[1])
				inHunk = true
				continue
			}
			if !inHunk || line == "" {
				continue
			}
			switch line[0] {
			case '+':
				lines[n] = true
				n++
			case ' ':
				n++
				oldLine++
			case '-':
				removed[oldLine] = true
				oldLine++
			case '\\':
			default:
				inHunk = false
			}
		}
		if !c.Deleted {
			if d.Added[p] == nil {
				d.Added[p] = map[int]bool{}
			}
			for line, added := range lines {
				d.Added[p][line] = added
			}
		}
		if d.Removed[oldPath] == nil {
			d.Removed[oldPath] = map[int]bool{}
		}
		for line, deleted := range removed {
			d.Removed[oldPath][line] = deleted
		}
	}
	d.Text = out.String()
	return d
}
