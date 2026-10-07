package platform

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hmarr/codeowners"
)

func TestGitHubCodeOwnerCompatibility(t *testing.T) {
	raw := "# comment\r\n* @default\r\n/源码/ @组织/team\n"
	if rules, err := ParseCodeOwnerRules("github", raw); err == nil || rules != nil {
		t.Fatal("invalid account returned partial rules")
	}
	raw = "# comment\r\n* @default\r\n/源码/ @org/team audit@example.technology\n/space\\ dir/ O'Connor@example.travel\n/config+v2.yaml owner+tag@example.engineering\n/ @root\n/源码/private/\n"
	rules, err := ParseCodeOwnerRules("github", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		line   int
		owners []string
	}{
		{"源码/main.go", 3, []string{"@org/team", "audit@example.technology"}},
		{"space dir/file", 4, []string{"O'Connor@example.travel"}},
		{"config+v2.yaml", 5, []string{"owner+tag@example.engineering"}},
		{"other", 2, []string{"@default"}},
		{"源码/private/key", 7, []string{}},
	} {
		matches, err := rules.Match(tc.path)
		if err != nil || len(matches.Rules) != 1 {
			t.Fatalf("%s: %v %v", tc.path, matches, err)
		}
		got := matches.Rules[0]
		if got.Line != tc.line || !reflect.DeepEqual(got.Owners, tc.owners) {
			t.Fatalf("%s: %+v", tc.path, got)
		}
	}
	root, err := ParseCodeOwnerRules("github", "/ @owner")
	if err != nil {
		t.Fatal(err)
	}
	matches, err := root.Match("file")
	if err != nil || len(matches.Rules) != 0 {
		t.Fatalf("root: %v %v", matches, err)
	}
	// Returned evidence must not expose mutable parser state.
	matches, _ = rules.Match("other")
	matches.Rules[0].Owners[0] = "@changed"
	matches, _ = rules.Match("other")
	if matches.Rules[0].Owners[0] != "@default" {
		t.Fatal("mutable owners")
	}
}

func TestGitHubCodeOwnerDifferential(t *testing.T) {
	patterns := []string{"*", "**", "*.go", "/main.go", "docs/", "/docs/", "docs/*", "docs/**", "**/main.go", "a/**/b", "a?b", "/foo", "foo/bar", "a\\ b/"}
	paths := []string{"main.go", "nested/main.go", "docs/file", "docs/nested/file", "docs", "other/docs/file", "a/b", "a/x/b", "a/x/y/b", "axb", "a字b", "foo", "foo/bar", "foo/bar/x", "other/foo", "a b/file"}
	for _, pattern := range patterns {
		raw := pattern + " @owner\n"
		old, err := codeowners.ParseFile(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		current, err := ParseCodeOwnerRules("github", raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			want, err := old.Match(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := current.Match(path)
			if err != nil || (len(got.Rules) > 0) != (want != nil) {
				t.Fatalf("pattern %q path %q: %v != %v, %v", pattern, path, got, want, err)
			}
		}
	}
}

func TestGitHubCodeOwnerRejectsPartialAndBudgets(t *testing.T) {
	for _, bad := range []string{"!private @owner", "[ab] @owner", "\\#file @owner", "foo#bar @owner", "foo\\", "foo @bad/name/extra", "foo display<owner@example.com>", "foo owner#tag@example.com", "foo\x01 @owner", "foo @owner\x01", "foo ***", "*** @owner", strings.Repeat("a", 1025) + " @owner", "foo " + strings.Repeat("@owner ", 101), strings.Repeat("foo @owner\n", 2049), strings.Repeat("foo "+strings.Repeat("@owner ", 100)+"\n", 82), strings.Repeat("#\n", 4096), strings.Repeat("#", 4097), strings.Repeat("#", 256*1024+1)} {
		rules, err := ParseCodeOwnerRules("github", "* @default\n"+bad)
		if err == nil || rules != nil {
			t.Fatalf("accepted invalid input (%d bytes): %.60q", len(bad), bad)
		}
	}
	for _, raw := range []string{strings.Repeat("a", 1024) + " @owner", "foo " + strings.Repeat("@owner ", 100), strings.Repeat("foo\n", 2048)} {
		if _, err := ParseCodeOwnerRules("github", raw); err != nil {
			t.Fatalf("rejected boundary: %v", err)
		}
	}
}
