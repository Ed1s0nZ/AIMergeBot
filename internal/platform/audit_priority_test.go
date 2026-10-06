package platform

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestRiskPriorityKeepsUnknownLanguagesAndChangedLines(t *testing.T) {
	neutral := Change{NewPath: "unknown.未登记", Diff: "@@ -1 +1 @@\n stable auth token exec\n-old\n+new"}
	if changePriority(neutral) != 1 {
		t.Fatal("unchanged context became priority")
	}
	guarded := Change{NewPath: "z/handler.任意", Diff: "@@ -1 +1 @@\n-if permission(owner, request) { guard(); }\n+exec(payload)"}
	if changePriority(guarded) != 3 {
		t.Fatal("removed protections/input/sink signals missed")
	}
	changes := []Change{neutral, guarded}
	for i := 0; i < 50; i++ {
		changes = append(changes, Change{NewPath: fmt.Sprintf("a/file%02d.any", i), Diff: "@@ -1 +1 @@\n-old\n+new"})
	}
	plan := PlanAuditGroups(changes, nil)
	if plan.Groups[0].Files[0] != guarded.NewPath || plan.Groups[0].PriorityWeight != 3 {
		t.Fatal("risk hints did not receive first inspection")
	}
	seen := false
	for _, g := range plan.Groups {
		for _, f := range g.Files {
			seen = seen || f == neutral.NewPath
		}
	}
	if !seen {
		t.Fatal("unknown language excluded")
	}
	for i, j := 0, len(changes)-1; i < j; i, j = i+1, j-1 {
		changes[i], changes[j] = changes[j], changes[i]
	}
	other := PlanAuditGroups(changes, nil)
	a, _ := json.Marshal(plan)
	b, _ := json.Marshal(other)
	if string(a) != string(b) {
		t.Fatal("input order changed scheduling")
	}
	excluded := PlanAuditGroups([]Change{{NewPath: "notes.md", Diff: "@@ -1 +1 @@\n+exec(request, auth)"}}, []string{"md"})
	if len(excluded.Groups) != 0 || len(excluded.Excluded) != 1 {
		t.Fatal("priority bypassed exclusions")
	}
}

func TestPriorityBudgetsReserveNormalGroupsAndStayBounded(t *testing.T) {
	groups := []AuditGroup{{PriorityWeight: 3}, {PriorityWeight: 1}, {PriorityWeight: 1}}
	if got := priorityCallBudget(80, groups, 2); got != 46 {
		t.Fatal("wrong weighted allocation", got)
	}
	if got := priorityCallBudget(80, []AuditGroup{{}, {}, {}}, 2); got != 26 {
		t.Fatal("neutral allocation changed", got)
	}
	for _, budget := range []int{0, 1, 2, 3, 4, 8, 80, math.MaxInt} {
		remaining := budget
		for i := range groups {
			calls := priorityCallBudget(remaining, groups[i:], 2)
			if calls < 0 || calls > remaining {
				t.Fatal("budget overrun", budget, calls, remaining)
			}
			if remaining/2 >= len(groups)-i && calls < 2 {
				t.Fatal("normal group starved")
			}
			remaining -= calls
		}
	}
	if priorityCallBudget(1, groups, 2) != 0 || priorityCallBudget(2, groups, 2) != 2 || priorityCallBudget(2, groups, 3) != 0 || priorityCallBudget(3, groups, 3) != 3 {
		t.Fatal("minimum effective preflight/source budget violated")
	}
	if priorityCallBudget(80, nil, 2) != 0 {
		t.Fatal("empty group allocation")
	}
	if got := priorityTimeBudget(100*time.Second, groups); got != 60*time.Second {
		t.Fatal("wrong time allocation", got)
	}
	if got := priorityTimeBudget(time.Duration(math.MaxInt), groups); got <= 0 || got > time.Duration(math.MaxInt) {
		t.Fatal("duration overflow", got)
	}
	if groupPriority(AuditGroup{PriorityWeight: 100}) != 3 || groupPriority(AuditGroup{PriorityWeight: -1}) != 1 {
		t.Fatal("unbounded/historical weight")
	}
}

func TestPriorityPlanningKeepsOmissionCoverage(t *testing.T) {
	changes := []Change{}
	for i := 0; i < 200; i++ {
		changes = append(changes, Change{NewPath: fmt.Sprintf("a/f%03d.any", i), Diff: "@@ -1 +1 @@\n+normal"})
	}
	changes = append(changes, Change{NewPath: "z/entry.unknown", Diff: "@@ -1 +1 @@\n+exec(request, auth)"})
	plan := PlanAuditGroups(changes, nil)
	if len(plan.Groups) != auditMaxGroups || plan.Groups[0].Files[0] != "z/entry.unknown" || len(plan.OmittedFiles) != 9 {
		t.Fatal("priority silently lost scope", plan.OmittedFiles)
	}
	for _, g := range plan.Groups {
		if len(g.Scope.Text) > auditGroupBytes || len(g.Files) > auditGroupFiles {
			t.Fatal("group limits changed")
		}
	}
	if !strings.Contains(currentGroupNavigation(&plan.Groups[0]), "not source evidence") {
		t.Fatal("priority became evidence")
	}
}
