package platform

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

var gitLabOwnerHeader = regexp.MustCompile(`^(\^)?\[([^\]]+)\](?:\[([0-9]+)\])?(?:\s+(.*))?$`)
var gitLabOwnerIdentity = regexp.MustCompile(`^(?:@[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*|@@(?i:developers?|maintainers?|owners?)|[^@\s]{1,100}@[^@\s]{1,255})$`)

var gitLabOwnerNameReference = regexp.MustCompile(`(?:^|[^A-Za-z0-9_@])(@[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)`)
var gitLabOwnerRoleReference = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_@])(@@(?:developers?|maintainers?|owners?))`)
var gitLabOwnerEmailReference = regexp.MustCompile(`[^@\s]{1,100}@[^@\s]{0,254}[A-Za-z0-9_]`)

func gitLabOwnerTokens(text string) ([]string, error) {
	owners := []string{}
	seen := map[string]bool{}
	appendOwner := func(owner string) error {
		if len(owner) > 256 {
			return ErrCodeOwnersInput
		}
		if !seen[owner] {
			seen[owner] = true
			owners = append(owners, owner)
		}
		return nil
	}
	// GitLab extracts references from the entire owner text, including comments.
	// Preserve name/email/role categories in the vendor's extraction order.
	for _, match := range gitLabOwnerNameReference.FindAllStringSubmatch(text, -1) {
		if err := appendOwner(match[1]); err != nil {
			return nil, err
		}
	}
	for _, match := range gitLabOwnerRoleReference.FindAllStringSubmatchIndex(text, -1) {
		if match[3] < len(text) && !strings.ContainsRune(" \t\r\n", rune(text[match[3]])) {
			continue
		}
		if err := appendOwner(text[match[2]:match[3]]); err != nil {
			return nil, err
		}
	}
	for _, owner := range gitLabOwnerEmailReference.FindAllString(text, -1) {
		if err := appendOwner(owner); err != nil {
			return nil, err
		}
	}
	if len(owners) > 100 {
		return nil, ErrCodeOwnersInput
	}
	return owners, nil
}
func splitGitLabOwnerEntry(line string) (string, string) {
	escaped := false
	for i, ch := range line {
		if (ch == ' ' || ch == '\t') && !escaped {
			return line[:i], strings.TrimSpace(line[i:])
		}
		if ch == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	return line, ""
}
func normalizeGitLabOwnerGlob(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "\\ ", " ")
	pattern = strings.ReplaceAll(pattern, "\\\t", " ")
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/**/" + pattern
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**/*"
	}
	// Ruby fnmatch treats a trailing '**' as a single-segment wildcard.
	if strings.HasSuffix(pattern, "/**") {
		pattern = strings.TrimSuffix(pattern, "**") + "*"
	}
	// Ruby FNM_EXTGLOB is disabled; braces are literals, not alternation.
	var literal strings.Builder
	escaped := false
	for _, ch := range pattern {
		if (ch == '{' || ch == '}') && !escaped {
			literal.WriteByte('\\')
		}
		literal.WriteRune(ch)
		if ch == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	return literal.String()
}
func (r *CodeOwnerRules) parseGitLab(lines []string) error {
	section := "codeowners"
	optional := false
	approvals := 0
	defaults := []string{}
	sectionOptional := map[string]bool{"codeowners": false}
	sectionNames := map[string]string{"codeowners": "codeowners"}
	total := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") || strings.HasPrefix(line, "^[") {
			header := gitLabOwnerHeader.FindStringSubmatch(line)
			if header == nil || strings.TrimSpace(header[2]) == "" || len(header[2]) > 128 {
				return ErrCodeOwnersInput
			}
			lower := strings.ToLower(header[2])
			if _, ok := sectionNames[lower]; !ok {
				sectionNames[lower] = header[2]
			}
			section = sectionNames[lower]
			optional = header[1] != ""
			approvals = 0
			if header[3] != "" {
				n, err := strconv.Atoi(header[3])
				if err != nil || n < 1 || n > 1000 || optional {
					return ErrCodeOwnersInput
				}
				approvals = n
			}
			owners, err := gitLabOwnerTokens(header[4])
			if err != nil {
				return err
			}
			for _, token := range strings.Fields(header[4]) {
				if !gitLabOwnerIdentity.MatchString(token) {
					return ErrCodeOwnersInput
				}
			}
			defaults = owners
			if previous, ok := sectionOptional[section]; ok {
				sectionOptional[section] = previous && optional
			} else {
				sectionOptional[section] = optional
			}
			continue
		}
		pattern, text := splitGitLabOwnerEntry(line)
		excluded := strings.HasPrefix(pattern, "!")
		if excluded {
			pattern = pattern[1:]
		}
		if pattern == "" || len(pattern) > 1024 {
			return ErrCodeOwnersInput
		}
		normalized := pattern
		if strings.HasPrefix(normalized, "\\#") {
			normalized = "#" + normalized[2:]
		}
		if normalized == "*" {
			normalized = "/**/*"
		} else {
			normalized = normalizeGitLabOwnerGlob(normalized)
		}
		if !doublestar.ValidatePattern(normalized) {
			return ErrCodeOwnersInput
		}
		owners, err := gitLabOwnerTokens(text)
		if err != nil {
			return err
		}
		if !excluded {
			if text == "" {
				owners = append([]string{}, defaults...)
			}
			if len(owners) == 0 {
				return ErrCodeOwnersInput
			}
		} else {
			owners = []string{}
		}
		total += len(owners)
		if total > 8192 {
			return ErrCodeOwnersInput
		}
		// The vendor replaces identical normalized patterns and moves them to the end.
		for j := 0; j < len(r.rules); j++ {
			if r.rules[j].Section == section && r.rules[j].normalized == normalized {
				r.rules = append(r.rules[:j], r.rules[j+1:]...)
				break
			}
		}
		r.rules = append(r.rules, CodeOwnerRule{Line: i + 1, Pattern: pattern, Section: section, Owners: owners, Excluded: excluded, Optional: optional, Approvals: approvals, normalized: normalized})
		if len(r.rules) > 2048 {
			return ErrCodeOwnersInput
		}
	}
	for i := range r.rules {
		if r.rules[i].Section != "" {
			r.rules[i].Optional = sectionOptional[r.rules[i].Section]
		}
	}
	return nil
}
