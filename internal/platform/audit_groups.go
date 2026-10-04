package platform

import (
	"fmt"
	"path"
	"sort"
)

const (
	auditGroupBytes     = 32 * 1024
	auditGroupFiles     = 24
	auditMaxGroups      = 8
	auditTotalDiffBytes = 96 * 1024
)

// AuditGroup is a planning boundary, not a semantic dependency assertion.
type AuditGroup struct {
	ID    string    `json:"id"`
	Files []string  `json:"files"`
	Scope DiffScope `json:"-"`
}
type AuditPlan struct {
	Groups   []AuditGroup `json:"groups"`
	Excluded []string     `json:"excluded"`
	Notes    []string     `json:"notes"`
}

func changePath(c Change) string {
	if c.Deleted {
		return c.OldPath
	}
	return c.NewPath
}

// PlanAuditGroups keeps complete file sections. Any omitted source remains an
// explicit coverage gap; limits never certify the omitted files as reviewed.
func PlanAuditGroups(changes []Change, excluded []string) AuditPlan {
	ordered := append([]Change(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := changePath(ordered[i]), changePath(ordered[j])
		if path.Dir(a) != path.Dir(b) {
			return path.Dir(a) < path.Dir(b)
		}
		return a < b
	})
	plan := AuditPlan{Groups: []AuditGroup{}, Excluded: []string{}, Notes: []string{}}
	selected := []Change{}
	files := []string{}
	groupBytes, totalBytes := 0, 0
	flush := func() {
		if len(selected) == 0 {
			return
		}
		scope := BuildDiff(selected, nil, auditGroupBytes)
		scope.Notes = nil // Input gaps are collected once in plan.Notes.
		plan.Groups = append(plan.Groups, AuditGroup{ID: fmt.Sprintf("group-%d", len(plan.Groups)+1), Files: append([]string{}, files...), Scope: scope})
		selected = nil
		files = nil
		groupBytes = 0
	}
	for _, c := range ordered {
		single := BuildDiff([]Change{c}, excluded, auditGroupBytes)
		plan.Excluded = append(plan.Excluded, single.Excluded...)
		plan.Notes = append(plan.Notes, single.Notes...)
		if single.Text == "" {
			continue
		}
		size := len(single.Text)
		if totalBytes+size > auditTotalDiffBytes {
			plan.Notes = append(plan.Notes, "Grouped diff total budget exceeded; omitted: "+changePath(c))
			continue
		}
		if len(selected) >= auditGroupFiles || groupBytes+size > auditGroupBytes {
			flush()
		}
		if len(plan.Groups) >= auditMaxGroups {
			plan.Notes = append(plan.Notes, "Audit group count exceeded; omitted: "+changePath(c))
			continue
		}
		selected = append(selected, c)
		files = append(files, changePath(c))
		groupBytes += size
		totalBytes += size
	}
	flush()
	return plan
}
