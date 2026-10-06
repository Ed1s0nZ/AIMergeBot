package platform

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	auditGroupBytes     = 32 * 1024
	auditGroupFiles     = 24
	auditMaxGroups      = 8
	auditTotalDiffBytes = auditMaxGroups * auditGroupBytes
)

// AuditGroup is a planning boundary, not a semantic dependency assertion.
type AuditGroup struct {
	PriorityWeight int       `json:"priority_weight"`
	ID             string    `json:"id"`
	Files          []string  `json:"files"`
	Scope          DiffScope `json:"-"`
}
type AuditPlan struct {
	Groups       []AuditGroup `json:"groups"`
	Excluded     []string     `json:"excluded"`
	Notes        []string     `json:"notes"`
	OmittedFiles []string     `json:"omitted_files"`
}

func changePath(c Change) string {
	if c.Deleted {
		return c.OldPath
	}
	return c.NewPath
}

// PlanAuditGroups keeps whole source lines in fixed-coordinate chunks. Any omitted source remains an
// explicit coverage gap; limits never certify the omitted files as reviewed.
func PlanAuditGroups(changes []Change, excluded []string) AuditPlan {
	ordered := append([]Change(nil), changes...)
	priorities := map[string]int{}
	for _, c := range ordered {
		if weight := changePriority(c); weight > priorities[changePath(c)] {
			priorities[changePath(c)] = weight
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := changePath(ordered[i]), changePath(ordered[j])
		if priorities[a] != priorities[b] {
			return priorities[a] > priorities[b]
		}
		if path.Dir(a) != path.Dir(b) {
			return path.Dir(a) < path.Dir(b)
		}
		return a < b
	})
	plan := AuditPlan{Groups: []AuditGroup{}, Excluded: []string{}, Notes: []string{}, OmittedFiles: []string{}}
	selected := []Change{}
	files := []string{}
	groupBytes, totalBytes := 0, 0
	groupWeight := 1
	flush := func() {
		if len(selected) == 0 {
			return
		}
		scope := BuildDiff(selected, nil, auditGroupBytes)
		scope.Notes = nil // Input gaps are collected once in plan.Notes.
		plan.Groups = append(plan.Groups, AuditGroup{PriorityWeight: groupWeight, ID: fmt.Sprintf("group-%d", len(plan.Groups)+1), Files: append([]string{}, files...), Scope: scope})
		selected = nil
		files = nil
		groupBytes = 0
		groupWeight = 1
	}
	for _, c := range ordered {
		single := BuildDiff([]Change{c}, excluded, auditGroupBytes)
		plan.Excluded = append(plan.Excluded, single.Excluded...)
		units := []Change{c}
		if len(single.Excluded) == 0 && validPath(changePath(c)) && single.Text == "" && c.Diff != "" && !strings.HasPrefix(c.Diff, "Binary files") {
			var chunkNotes []string
			units, chunkNotes = splitChangeDiff(c, auditGroupBytes)
			for _, note := range single.Notes {
				if !strings.HasPrefix(note, "Diff budget exceeded;") {
					plan.Notes = append(plan.Notes, note)
				}
			}
			plan.Notes = append(plan.Notes, chunkNotes...)
			if len(chunkNotes) > 0 {
				plan.OmittedFiles = append(plan.OmittedFiles, changePath(c))
			}
		} else {
			plan.Notes = append(plan.Notes, single.Notes...)
			if single.Text == "" {
				continue
			}
		}
		for _, unit := range units {
			input := BuildDiff([]Change{unit}, nil, auditGroupBytes)
			size := len(input.Text)
			if size == 0 {
				continue
			}
			if totalBytes+size > auditTotalDiffBytes {
				plan.Notes = append(plan.Notes, "Grouped diff total budget exceeded; omitted chunk: "+changePath(c))
				plan.OmittedFiles = append(plan.OmittedFiles, changePath(c))
				continue
			}
			if len(selected) >= auditGroupFiles || groupBytes+size > auditGroupBytes {
				flush()
			}
			if len(plan.Groups) >= auditMaxGroups {
				plan.Notes = append(plan.Notes, "Audit group count exceeded; omitted chunk: "+changePath(c))
				plan.OmittedFiles = append(plan.OmittedFiles, changePath(c))
				continue
			}
			selected = append(selected, unit)
			if priorities[changePath(c)] > groupWeight {
				groupWeight = priorities[changePath(c)]
			}
			if len(files) == 0 || files[len(files)-1] != changePath(c) {
				files = append(files, changePath(c))
			}
			groupBytes += size
			totalBytes += size
		}
	}
	flush()
	return plan
}
