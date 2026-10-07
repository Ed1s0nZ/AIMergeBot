package platform

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	"github.com/hmarr/codeowners"
)

type githubCodeOwnerRule struct {
	rule    CodeOwnerRule
	pattern *regexp.Regexp
}

func (r *CodeOwnerRules) parseGitHub(lines []string) error {
	total := 0
	for i, raw := range lines {
		line := strings.TrimLeft(strings.TrimSuffix(raw, "\r"), " \t")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		end, escaped := len(line), false
		for pos, ch := range line {
			if unicode.IsControl(ch) && ch != '\t' {
				return ErrCodeOwnersInput
			}
			if escaped {
				if ch == '/' || ch == '\t' || (pos == 1 && ch == '#') {
					return ErrCodeOwnersInput
				}
				escaped = false
				continue
			}
			if ch == ' ' || ch == '\t' {
				end = pos
				break
			}
			if ch == '#' || ch == '[' || ch == ']' || (pos == 0 && ch == '!') {
				return ErrCodeOwnersInput
			}
			if ch == '\\' {
				escaped = true
			}
		}
		if escaped || end == 0 || end > 1024 {
			return ErrCodeOwnersInput
		}
		pattern := line[:end]
		compiled, err := compileGitHubCodeOwnerPattern(pattern)
		if err != nil {
			return ErrCodeOwnersInput
		}
		owners := []string{}
		for _, token := range strings.FieldsFunc(line[end:], func(ch rune) bool { return ch == ' ' || ch == '\t' }) {
			if strings.HasPrefix(token, "#") {
				break
			}
			if !validGitHubCodeOwner(token) {
				return ErrCodeOwnersInput
			}
			owners = append(owners, token)
			if len(owners) > 100 {
				return ErrCodeOwnersInput
			}
		}
		total += len(owners)
		if total > 8192 || len(r.github) >= 2048 {
			return ErrCodeOwnersInput
		}
		r.github = append(r.github, githubCodeOwnerRule{
			rule: CodeOwnerRule{Line: i + 1, Pattern: pattern, Owners: owners}, pattern: compiled,
		})
	}
	return nil
}

func validGitHubCodeOwner(token string) bool {
	if strings.ContainsRune(token, '#') {
		return false
	}
	for _, ch := range token {
		if unicode.IsControl(ch) || unicode.IsSpace(ch) {
			return false
		}
	}
	if strings.HasPrefix(token, "@") {
		_, user := codeowners.MatchUsernameOwner(token)
		_, team := codeowners.MatchTeamOwner(token)
		return user == nil || team == nil
	}
	address, err := mail.ParseAddress(token)
	return err == nil && address.Name == "" && address.Address == token
}
