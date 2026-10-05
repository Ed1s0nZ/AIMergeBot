package platform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var chunkHunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?:.*)$`)

// splitChangeDiff only rewrites oversized unified diffs. Whole source lines
// retain their original coordinates; omitted input is always a coverage gap.
func splitChangeDiff(c Change, maxBytes int) ([]Change, []string) {
	single := BuildDiff([]Change{c}, nil, maxBytes)
	if single.Text != "" {
		return []Change{c}, nil
	}
	p := changePath(c)
	header := c
	header.Diff = "@@ -1,0 +1,0 @@\n"
	// Reserve enough space for all four decimal coordinates in a chunk header.
	overhead := len(BuildDiff([]Change{header}, nil, 1<<30).Text) + 96
	payloadLimit := maxBytes - overhead
	if payloadLimit < 1 {
		return nil, []string{"Diff chunk header exceeds budget: " + p}
	}
	lines := strings.Split(c.Diff, "\n")
	chunks := []Change{}
	notes := []string{}
	emit := func(body []string, old, new, oldCount, newCount int) {
		if len(body) == 0 {
			return
		}
		if oldCount == 0 {
			old--
		}
		if newCount == 0 {
			new--
		}
		v := c
		v.Notes = nil
		v.Diff = fmt.Sprintf("@@ -%d,%d +%d,%d @@\n%s\n", old, oldCount, new, newCount, strings.Join(body, "\n"))
		if len(BuildDiff([]Change{v}, nil, maxBytes).Text) == 0 {
			notes = append(notes, "Diff chunk exceeds budget; omitted: "+p)
			return
		}
		chunks = append(chunks, v)
	}
	found := false
	for i := 0; i < len(lines); {
		m := chunkHunkPattern.FindStringSubmatch(lines[i])
		if m == nil {
			if lines[i] != "" {
				notes = append(notes, "Malformed diff content omitted: "+p)
			}
			i++
			continue
		}
		found = true
		old, oldErr := strconv.Atoi(m[1])
		new, newErr := strconv.Atoi(m[3])
		validNumbers := oldErr == nil && newErr == nil
		oldExpected, newExpected := 1, 1
		if m[2] != "" {
			var err error
			oldExpected, err = strconv.Atoi(m[2])
			validNumbers = validNumbers && err == nil
		}
		if m[4] != "" {
			var err error
			newExpected, err = strconv.Atoi(m[4])
			validNumbers = validNumbers && err == nil
		}
		j := i + 1
		oldSeen, newSeen := 0, 0
		maxInt := int(^uint(0) >> 1)
		valid := validNumbers && old >= 0 && new >= 0 && oldExpected >= 0 && newExpected >= 0 && old < maxInt-oldExpected && new < maxInt-newExpected
		for j < len(lines) && chunkHunkPattern.FindStringSubmatch(lines[j]) == nil {
			line := lines[j]
			if line == "" && j == len(lines)-1 {
				j++
				break
			}
			if line == "" {
				valid = false
			} else {
				switch line[0] {
				case ' ':
					oldSeen++
					newSeen++
				case '-':
					oldSeen++
				case '+':
					newSeen++
				case '\\':
					if line != `\ No newline at end of file` {
						valid = false
					}
				default:
					valid = false
				}
			}
			j++
		}
		if !valid || oldSeen != oldExpected || newSeen != newExpected {
			notes = append(notes, "Malformed diff hunk omitted: "+p)
			i = j
			continue
		}
		if oldExpected == 0 {
			old++
		}
		if newExpected == 0 {
			new++
		}
		body := []string{}
		size, oc, nc := 0, 0, 0
		startOld, startNew := old, new
		flush := func() {
			emit(body, startOld, startNew, oc, nc)
			body = nil
			size, oc, nc = 0, 0, 0
			startOld, startNew = old, new
		}
		for _, line := range lines[i+1 : j] {
			if line == "" {
				continue
			}
			if size+len(line)+1 > payloadLimit {
				flush()
			}
			if len(line)+1 > payloadLimit {
				notes = append(notes, "Oversized diff line omitted: "+p)
			} else if line[0] != '\\' || len(body) > 0 {
				body = append(body, line)
				size += len(line) + 1
				if line[0] == ' ' || line[0] == '-' {
					oc++
				}
				if line[0] == ' ' || line[0] == '+' {
					nc++
				}
			} else {
				notes = append(notes, "Diff newline marker omitted at chunk boundary: "+p)
			}
			if line[0] == ' ' || line[0] == '-' {
				old++
			}
			if line[0] == ' ' || line[0] == '+' {
				new++
			}
			if len(body) == 0 {
				startOld, startNew = old, new
			}
		}
		flush()
		i = j
	}
	if !found {
		notes = append(notes, "No valid unified diff hunk available for chunking: "+p)
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, note := range notes {
		if !seen[note] {
			unique = append(unique, note)
			seen[note] = true
		}
	}
	return chunks, unique
}

func mergeScopeAnchors(dst *DiffScope, src DiffScope) {
	for p, lines := range src.Added {
		if dst.Added[p] == nil {
			dst.Added[p] = map[int]bool{}
		}
		for n, added := range lines {
			dst.Added[p][n] = added
		}
	}
	for p, lines := range src.Removed {
		if dst.Removed[p] == nil {
			dst.Removed[p] = map[int]bool{}
		}
		for n, removed := range lines {
			dst.Removed[p][n] = removed
		}
	}
	for p, metadata := range src.Metadata {
		dst.Metadata[p] = metadata
	}
}
