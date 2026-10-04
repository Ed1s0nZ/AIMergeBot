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
	Removed map[string]map[int]bool
	Text    string
	Added   map[string]map[int]bool
	Notes   []string
}

func BuildDiff(changes []Change, excluded []string, maxBytes int) DiffScope {
	d := DiffScope{Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Notes: []string{}}
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
			d.Notes = append(d.Notes, "Excluded by extension: "+p)
			continue
		}
		if !validPath(p) {
			d.Notes = append(d.Notes, "Invalid path omitted")
			continue
		}
		if c.Diff == "" || strings.HasPrefix(c.Diff, "Binary files") {
			d.Notes = append(d.Notes, "No textual diff available: "+p)
			continue
		}
		section := fmt.Sprintf("File: %s (old: %s, deleted: %t, renamed: %t)\n%s\n", p, c.OldPath, c.Deleted, c.Renamed, c.Diff)
		if out.Len()+len(section) > maxBytes {
			d.Notes = append(d.Notes, "Diff budget exceeded; omitted: "+p)
			continue
		}
		out.WriteString(section)
		lines := map[int]bool{}
		removed := map[int]bool{}
		oldPath := c.OldPath
		if oldPath == "" {
			oldPath = p
		}
		oldLine := 0
		n := 0
		inHunk := false
		for _, line := range strings.Split(c.Diff, "\n") {
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
			d.Added[p] = lines
		}
		d.Removed[oldPath] = removed
	}
	d.Text = out.String()
	return d
}
