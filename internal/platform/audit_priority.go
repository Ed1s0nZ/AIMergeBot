package platform

import (
	"regexp"
	"strings"
	"time"
)

// Lexical hints allocate inspection effort; they never establish a risk or a safe file.
// No extension or language is required. Missing hints keep the normal inspection weight.
var auditPriorityHints = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(auth[a-z_]*|permission[a-z_]*|principal|tenant|owner[a-z_]*|role[a-z_]*|token|session|csrf|guard|allowlist|denylist)\b`),
	regexp.MustCompile(`(?i)\b(request|headers?|query|input|params?|parameters?|upload|webhook|cookie|payload)\b`),
	regexp.MustCompile(`(?i)\b(exec[a-z_]*|spawn|system|shell|eval|deserializ[a-z_]*|unserialize|redirect|sql|select|insert|delete|write|fetch|socket)\b`),
}

func changePriority(c Change) int {
	// Bound planning cost for a very large untrusted patch. Unscanned text is not certified safe.
	text := c.Diff
	if len(text) > auditTotalDiffBytes {
		text = text[:auditTotalDiffBytes]
	}
	var changed strings.Builder
	changed.WriteString(changePath(c))
	for _, line := range strings.Split(text, "\n") {
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') && !strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "---") {
			changed.WriteByte('\n')
			changed.WriteString(line[1:])
		}
	}
	weight := 1
	for _, hint := range auditPriorityHints {
		if hint.MatchString(changed.String()) {
			weight++
		}
		if weight == 3 {
			break
		}
	}
	return weight
}
func groupPriority(g AuditGroup) int {
	if g.PriorityWeight < 1 {
		return 1
	}
	if g.PriorityWeight > 3 {
		return 3
	}
	return g.PriorityWeight
}
func remainingPriority(groups []AuditGroup) int {
	total := 0
	for _, g := range groups {
		total += groupPriority(g)
	}
	return total
}

// Reserve preflight plus a source call for every remaining group when possible.
// Insufficient budget leaves groups unprocessed instead of funding inventory only.
func priorityCallBudget(remaining int, groups []AuditGroup, minimum int) int {
	if minimum < 1 {
		minimum = 1
	}
	if remaining < minimum || len(groups) == 0 {
		return 0
	}
	if remaining/minimum < len(groups) {
		return minimum
	}
	extra := remaining - minimum*len(groups)
	weight, total := groupPriority(groups[0]), remainingPriority(groups)
	return minimum + (extra/total)*weight + (extra%total)*weight/total
}
func priorityTimeBudget(remaining time.Duration, groups []AuditGroup) time.Duration {
	if remaining <= 0 || len(groups) == 0 {
		return 0
	}
	weight, total := time.Duration(groupPriority(groups[0])), time.Duration(remainingPriority(groups))
	return remaining/total*weight + (remaining%total)*weight/total
}
