package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestCodeOwnersGitHubLastMatchAndEmptyOverride(t *testing.T) {
	r, err := ParseCodeOwnerRules("github", "* @default\n/docs/ @org/docs\n/docs/private/\n/docs/README.md @readme # @ignored\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file   string
		line   int
		owners string
	}{{"main.any", 1, "@default"}, {"docs/nested/file.any", 2, "@org/docs"}, {"docs/private/key.any", 3, ""}, {"docs/README.md", 4, "@readme"}, {"Docs/README.md", 1, "@default"}} {
		m, err := r.Match(tc.file)
		if err != nil || len(m.Rules) != 1 || m.Rules[0].Line != tc.line || strings.Join(m.Rules[0].Owners, ",") != tc.owners {
			t.Fatal(tc, m, err)
		}
	}
}
func TestCodeOwnersGitLabSectionsDefaultsExclusionsAndRelativePaths(t *testing.T) {
	raw := `* @global
[Docs] @org/docs
/docs/
/docs/README.md @readme # additional @inline
^[Ruby]
*.rb @ruby
!/config/**/*.rb
/config/routes.rb @must-not-reinclude
[Other]
/config/ @ops
[docs]
/docs/special.md @special
[Email][2] owner@example.test
internal/README.md
[Roles]
*.rb @@developers @@maintainer
`
	r, err := ParseCodeOwnerRules("gitlab", raw)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		file     string
		owners   string
		excluded int
	}{{"docs/README.md", "@global,@readme,@inline", 0}, {"docs/special.md", "@global,@special", 0}, {"config/routes.rb", "@global,@ops,@@developers,@@maintainer", 1}, {"config/deep/module.rb", "@global,@ops,@@developers,@@maintainer", 1}, {"x/internal/README.md", "@global,owner@example.test", 0}, {"src/module.rb", "@global,@ruby,@@developers,@@maintainer", 0}}
	for _, tc := range checks {
		m, err := r.Match(tc.file)
		owners := []string{}
		for _, rule := range m.Rules {
			owners = append(owners, rule.Owners...)
		}
		if err != nil || strings.Join(owners, ",") != tc.owners || len(m.Exclusions) != tc.excluded {
			t.Fatal(tc, m, err)
		}
		for _, rule := range m.Rules {
			if rule.Section == "Ruby" && !rule.Optional {
				t.Fatal("optional section lost", rule)
			}
			if rule.Section == "Email" && rule.Approvals != 2 {
				t.Fatal("approval metadata lost", rule)
			}
		}
	}
}
func TestCodeOwnersGitLabGlobSemantics(t *testing.T) {
	for _, tc := range []struct {
		pattern, file string
		match         bool
	}{{"/docs/*.md", "docs/nested/a.md", false}, {"/docs/**/*.md", "docs/a.md", true}, {"/docs/**/*.md", "docs/deep/a.md", true}, {"/docs/", "docs/deep/.hidden", true}, {"/docs/**", "docs/deep/a.md", false}, {"/docs/**", "docs/a.md", true}, {"internal/README.md", "x/internal/README.md", true}, {"/internal/README.md", "x/internal/README.md", false}, {"/a[bc].any", "ab.any", true}, {"/a[bc].any", "ad.any", false}, {"/file{a,b}", "filea", false}, {"/file{a,b}", "file{a,b}", true}, {`/file\{a,b\}`, "file{a,b}", true}, {"/a[^b].any", "ac.any", true}, {"/a[^b].any", "ab.any", false}, {`/space\ name/`, "space name/a.any", true}, {`\#config`, "#config", true}, {"/文档/", "文档/a.any", true}} {
		r, err := ParseCodeOwnerRules("gitlab", tc.pattern+" @owner")
		if err != nil {
			t.Fatal(tc, err)
		}
		m, err := r.Match(tc.file)
		if err != nil || (len(m.Rules) > 0) != tc.match {
			t.Fatal(tc, m, err)
		}
	}
}
func TestCodeOwnersGitLabDuplicatePatternsAndRequiredSections(t *testing.T) {
	r, err := ParseCodeOwnerRules("gitlab", "^[Docs] @default\n*.md\n[docs]\n/README.md @root\n*.md @last\n")
	if err != nil {
		t.Fatal(err)
	}
	m, err := r.Match("README.md")
	if err != nil || len(m.Rules) != 1 || m.Rules[0].Line != 5 || m.Rules[0].Optional || m.Rules[0].Owners[0] != "@last" {
		t.Fatal(m, err)
	}
}
func TestCodeOwnersBudgetsAndMalformedInputsNeverReturnPartialRules(t *testing.T) {
	for _, tc := range []struct{ provider, raw string }{{"other", "* @owner"}, {"github", strings.Repeat("x", 256*1024+1)}, {"gitlab", "* @owner\n" + strings.Repeat("x", 4097)}, {"gitlab", strings.Repeat("\n", 4096)}, {"gitlab", "* @owner\n[broken"}, {"gitlab", "[]\n* @owner"}, {"gitlab", "^[Docs][2]\n* @owner"}, {"gitlab", "* @owner\n/empty"}, {"github", "* @owner\n!private @other"}, {"github", "* @owner\n[range] @other"}, {"gitlab", "* @owner\x00"}, {"gitlab", string([]byte{0xff})}} {
		r, err := ParseCodeOwnerRules(tc.provider, tc.raw)
		if r != nil || !errors.Is(err, ErrCodeOwnersInput) {
			t.Fatal("partial or malformed accepted", tc.provider, r, err)
		}
	}
	r, err := ParseCodeOwnerRules("gitlab", "* @owner")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"../private", "/private", "a/../b", "", "a\nsecret", `a\b`} {
		if _, err := r.Match(file); !errors.Is(err, ErrCodeOwnersInput) {
			t.Fatal("invalid file accepted", file, err)
		}
	}
}

func TestCodeOwnersGitLabNamedDefaultSectionOverridesUnnamed(t *testing.T) {
	r, err := ParseCodeOwnerRules("gitlab", "* @default\n[codeowners]\n*.any @specific\n")
	if err != nil {
		t.Fatal(err)
	}
	m, err := r.Match("file.any")
	if err != nil || len(m.Rules) != 1 || m.Rules[0].Owners[0] != "@specific" {
		t.Fatal(m, err)
	}
}
