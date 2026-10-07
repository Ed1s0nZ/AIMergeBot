package platform

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFormattingHintsKeepLanguageSensitiveDiffs(t *testing.T) {
	cases := []struct {
		name, diff string
		want       bool
	}{
		{"python indentation", "@@ -1 +1 @@\n-    guard()\n+guard()", true},
		{"multiline string whitespace", "@@ -1 +1 @@\n-  string content\n+ string content", true},
		{"actual token change", "@@ -1 +1 @@\n-deny()\n+allow()", false},
		{"addition only", "@@ -0,0 +1 @@\n+guard()", false},
		{"identical", "@@ -1 +1 @@\n-guard()\n+guard()", false},
		{"context only", "@@ -1 +1 @@\n guard()", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changes := []Change{{NewPath: "fixture.py", Diff: tc.diff}}
			before := BuildDiff(changes, nil, 10000)
			hints := formattingHints(changes, before.Included)
			if (len(hints) > 0) != tc.want {
				t.Fatal(hints)
			}
			after := BuildDiff(changes, nil, 10000)
			if !reflect.DeepEqual(before, after) || !strings.Contains(after.Text, tc.diff) {
				t.Fatal("format hints removed audit evidence")
			}
			if len(formattingHints(changes, nil)) != 0 {
				t.Fatal("excluded file received hint")
			}
		})
	}
	for _, change := range []Change{{NewPath: "a.py", Deleted: true, Diff: cases[0].diff}, {NewPath: "a.py", Renamed: true, Diff: cases[0].diff}, {NewPath: "a.py", Diff: strings.Repeat(" ", 1048577)}} {
		if len(formattingHints([]Change{change}, []string{"a.py"})) != 0 {
			t.Fatal("unsupported change hinted")
		}
	}
}

type whitespaceHintRepo struct{ runRepo }

func (whitespaceHintRepo) Changes(context.Context, Snapshot) ([]Change, []string, error) {
	return []Change{{NewPath: "a.py", Diff: "@@ -1 +1 @@\n-    guard()\n+guard()"}}, nil, nil
}
func TestFormattingHintsAreOptionalWorkerAdvice(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Repository: whitespaceHintRepo{}, Auditor: immediateAuditor{}, Workers: 1, Timeout: 5 * time.Second}
	if err := runner.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer runner.Stop()
	first, _, err := runner.Submit(ctx, 1, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, first, "succeeded")
	old, err := s.Run(ctx, first)
	if err != nil || len(old.Result.FormatHints) != 0 {
		t.Fatal(old.Result, err)
	}
	if _, err = s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{FormatNoiseHints: true}); err != nil {
		t.Fatal(err)
	}
	second, _, err := runner.Submit(ctx, 1, 2, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, s, second, "succeeded")
	fresh, err := s.Run(ctx, second)
	if err != nil || len(fresh.Result.FormatHints) != 1 || fresh.Result.FormatHints[0].File != "a.py" || fresh.Result.Summary != "synthetic recovery completed" || len(fresh.Result.CoverageNotes) != 0 {
		t.Fatal("advice changed audit behavior", fresh.Result, err)
	}
}
