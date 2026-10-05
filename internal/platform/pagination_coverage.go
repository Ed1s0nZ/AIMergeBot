package platform

import (
	"encoding/json"
	"sort"
)

type paginationCoverage struct {
	Ranges map[int]int
	Total  int
}

// Called under t.mu. Only a continuous prefix reaching a terminal page is complete.
func (t *auditTools) paginationComplete(name, key string, args any, out toolOutput, failed bool) (bool, bool) {
	cursorBased := name == "search_repository_code" || name == "list_repository_directory" || name == "search_code" || name == "list_directory" || name == "get_diff" || name == "compare_files" || name == "get_history" || name == "search_history" || name == "git_blame"
	if !cursorBased && name != "list_files" {
		return false, false
	}
	if t.pages == nil {
		t.pages = map[string]*paginationCoverage{}
	}
	p := t.pages[key]
	if p == nil {
		p = &paginationCoverage{Ranges: map[int]int{}, Total: -1}
		t.pages[key] = p
	}
	raw, _ := json.Marshal(args)
	var fields struct {
		Cursor int `json:"cursor"`
		Page   int `json:"page"`
	}
	_ = json.Unmarshal(raw, &fields)
	start := fields.Cursor
	origin := 0
	end := start + len(out.Text)
	if name == "list_files" {
		origin = 1
		start = fields.Page
		if start == 0 {
			start = 1
		}
		end = start + 1
	}
	if !failed {
		if out.More && cursorBased {
			end = out.NextCursor
		}
		if end > start || !out.More {
			p.Ranges[start] = end
		}
		if !out.More {
			p.Total = end
		}
	}
	starts := []int{}
	for n := range p.Ranges {
		starts = append(starts, n)
	}
	sort.Ints(starts)
	covered := origin
	for _, n := range starts {
		if n > covered {
			break
		}
		if p.Ranges[n] > covered {
			covered = p.Ranges[n]
		}
	}
	return p.Total >= 0 && covered >= p.Total, true
}
