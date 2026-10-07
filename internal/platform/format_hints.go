package platform

import "strings"

type FormatHint struct {
	File         string `json:"file"`
	ChangedPairs int    `json:"changed_pairs"`
}

// Lexical hints never remove diff content or establish semantic equivalence.
func formattingHints(changes []Change, included []string) []FormatHint {
	allowed := map[string]bool{}
	for _, p := range included {
		allowed[p] = true
	}
	hints := []FormatHint{}
	for _, change := range changes {
		if len(hints) >= 200 {
			break
		}
		if !allowed[change.NewPath] || change.Deleted || change.Renamed || change.Metadata != nil || len(change.Diff) > 1048576 {
			continue
		}
		removed, added := []string{}, []string{}
		inHunk := false
		for _, line := range strings.Split(change.Diff, "\n") {
			if strings.HasPrefix(line, "@@ ") {
				inHunk = true
				continue
			}
			if !inHunk || len(line) == 0 {
				continue
			}
			switch line[0] {
			case '-':
				removed = append(removed, line[1:])
			case '+':
				added = append(added, line[1:])
			}
		}
		if len(removed) == 0 || len(removed) != len(added) {
			continue
		}
		different := false
		matches := true
		for i := range removed {
			if strings.TrimSpace(removed[i]) != strings.TrimSpace(added[i]) {
				matches = false
				break
			}
			different = different || removed[i] != added[i]
		}
		if matches && different {
			hints = append(hints, FormatHint{File: change.NewPath, ChangedPairs: len(added)})
		}
	}
	return hints
}
