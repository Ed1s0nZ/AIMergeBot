package platform

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
)

var ErrCodeOwnersInput = errors.New("CODEOWNERS input exceeds limits or has unsupported syntax")

type CodeOwnerRule struct {
	Line       int      `json:"line"`
	Pattern    string   `json:"pattern"`
	Section    string   `json:"section,omitempty"`
	Owners     []string `json:"owners"`
	Excluded   bool     `json:"excluded,omitempty"`
	Optional   bool     `json:"optional,omitempty"`
	Approvals  int      `json:"approvals,omitempty"`
	normalized string
}
type CodeOwnerMatches struct {
	Rules      []CodeOwnerRule `json:"rules"`
	Exclusions []CodeOwnerRule `json:"exclusions"`
}
type CodeOwnerRules struct {
	provider string
	github   []githubCodeOwnerRule
	rules    []CodeOwnerRule
}

// ParseCodeOwnerRules never returns a partial ruleset as a complete recommendation.
// Syntax errors and read budgets are surfaced to the caller, rather than skipped.
func ParseCodeOwnerRules(provider, raw string) (*CodeOwnerRules, error) {
	if len(raw) > 256*1024 || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return nil, ErrCodeOwnersInput
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > 4096 {
		return nil, ErrCodeOwnersInput
	}
	for _, line := range lines {
		if len(line) > 4096 {
			return nil, ErrCodeOwnersInput
		}
	}
	result := &CodeOwnerRules{provider: provider}
	switch provider {
	case "github":
		if err := result.parseGitHub(lines); err != nil {
			return nil, err
		}
	case "gitlab":
		if err := result.parseGitLab(lines); err != nil {
			return nil, err
		}
	default:
		return nil, ErrCodeOwnersInput
	}
	return result, nil
}

func (r *CodeOwnerRules) Match(file string) (CodeOwnerMatches, error) {
	result := CodeOwnerMatches{Rules: []CodeOwnerRule{}, Exclusions: []CodeOwnerRule{}}
	if !validPath(file) || !utf8.ValidString(file) || len(file) > 4096 || strings.ContainsAny(file, "\r\n\\") {
		return result, ErrCodeOwnersInput
	}
	if r.provider == "github" {
		for i := len(r.github) - 1; i >= 0; i-- {
			if r.github[i].pattern.MatchString(file) {
				rule := r.github[i].rule
				rule.Owners = append([]string{}, rule.Owners...)
				result.Rules = append(result.Rules, rule)
				break
			}
		}
		return result, nil
	}
	order := []string{}
	selected := map[string]CodeOwnerRule{}
	exclusions := map[string]CodeOwnerRule{}
	for _, rule := range r.rules {
		matched, err := doublestar.Match(rule.normalized, "/"+file)
		if err != nil {
			return result, ErrCodeOwnersInput
		}
		if !matched {
			continue
		}
		if _, ok := selected[rule.Section]; !ok {
			if _, excluded := exclusions[rule.Section]; !excluded {
				order = append(order, rule.Section)
			}
		}
		if rule.Excluded {
			exclusions[rule.Section] = rule
			delete(selected, rule.Section)
		} else if _, excluded := exclusions[rule.Section]; !excluded {
			selected[rule.Section] = rule
		}
	}
	for _, section := range order {
		if rule, ok := exclusions[section]; ok {
			result.Exclusions = append(result.Exclusions, rule)
		} else if rule, ok := selected[section]; ok {
			result.Rules = append(result.Rules, rule)
		}
	}
	return result, nil
}
