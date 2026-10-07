package platform

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormattingScopeClassification(t *testing.T) {
	cases := []struct {
		name            string
		diff            string
		hunks           int
		formattingHunks int
		formattingLines int
		addedLines      int
		removedLines    int
		anchorLine      int
		anchorText      string
	}{
		{
			name:            "indentation-only hunk",
			diff:            "@@ -1,3 +1,3 @@\n func main() {\n-    run()\n+        run()\n }",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
			anchorLine:      2,
			anchorText:      "        run()",
		},
		{
			name:            "mixed hunk is not pure",
			diff:            "@@ -1,3 +1,4 @@\n-    run()\n+        run()\n-old()\n+new()\n+extra()",
			hunks:           1,
			formattingHunks: 0,
			formattingLines: 1,
			addedLines:      3,
			removedLines:    2,
		},
		{
			name:            "whole-file reformat across hunks",
			diff:            "@@ -1,2 +1,2 @@\n-a\n+  a\n@@ -10,2 +10,2 @@\n-b\n+  b",
			hunks:           2,
			formattingHunks: 2,
			formattingLines: 2,
			addedLines:      2,
			removedLines:    2,
			anchorLine:      1,
			anchorText:      "  a",
		},
		{
			name:            "crlf-to-lf rewrite",
			diff:            "@@ -1,1 +1,1 @@\n-alpha\r\n+alpha",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
			anchorLine:      1,
			anchorText:      "alpha",
		},
		{
			name:            "internal spacing change",
			diff:            "@@ -1,1 +1,1 @@\n-x = 1\n+x=1",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
			anchorLine:      1,
			anchorText:      "x=1",
		},
		{
			name:            "added file is not formatting churn",
			diff:            "@@ -0,0 +1,2 @@\n+line one\n+line two",
			hunks:           1,
			formattingHunks: 0,
			formattingLines: 0,
			addedLines:      2,
			removedLines:    0,
		},
		{
			name:            "deleted file is not formatting churn",
			diff:            "@@ -1,2 +0,0 @@\n-line one\n-line two",
			hunks:           1,
			formattingHunks: 0,
			formattingLines: 0,
			addedLines:      0,
			removedLines:    2,
		},
		{
			name:            "string literal whitespace is a documented lexical limitation",
			diff:            "@@ -1,1 +1,1 @@\n-msg(\"a b\")\n+msg(\"ab\")",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
			anchorLine:      1,
			anchorText:      "msg(\"ab\")",
		},
		{
			name:            "raw-equal re-emitted line is not the anchor",
			diff:            "@@ -1,2 +1,2 @@\n-dupe\n+dupe\n-dupe  \n+  dupe",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      2,
			removedLines:    2,
			anchorLine:      2,
			anchorText:      "  dupe",
		},
		{
			name:            "no newline marker is ignored",
			diff:            "@@ -1,1 +1,1 @@\n-a\n+  a\n\\ No newline at end of file",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
			anchorLine:      1,
			anchorText:      "  a",
		},
		{
			name:            "blank-line whitespace change has no anchor",
			diff:            "@@ -1,1 +1,1 @@\n-  \n+",
			hunks:           1,
			formattingHunks: 1,
			formattingLines: 1,
			addedLines:      1,
			removedLines:    1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stat, ok := formattingScopeForDiff(c.diff)
			if !ok {
				t.Fatal("valid hunk not parsed")
			}
			if stat.Hunks != c.hunks || stat.FormattingHunks != c.formattingHunks || stat.FormattingLines != c.formattingLines || stat.AddedLines != c.addedLines || stat.RemovedLines != c.removedLines {
				t.Fatalf("stats mismatch: %+v", stat)
			}
			if c.anchorText == "" {
				if len(stat.Examples) != 0 {
					t.Fatalf("unexpected example: %+v", stat.Examples)
				}
				return
			}
			if len(stat.Examples) == 0 || stat.Examples[0].HeadLine != c.anchorLine || stat.Examples[0].Text != c.anchorText {
				t.Fatalf("anchor mismatch: %+v", stat.Examples)
			}
		})
	}
}

func TestFormattingScopeBuildDiffIntegration(t *testing.T) {
	changes := []Change{
		{NewPath: "a.go", OldPath: "a.go", Diff: "@@ -1,3 +1,3 @@\n func main() {\n-    run()\n+        run()\n }"},
		{NewPath: "b.go", OldPath: "b.go", Added: true, Diff: "@@ -0,0 +1,2 @@\n+line one\n+line two"},
		{NewPath: "c.go", OldPath: "c.go", Diff: "@@ -1,1 +1,1 @@\n-x = 1\n+x=1"},
		{NewPath: "d.go", OldPath: "d.go", Deleted: true, Diff: "@@ -1,1 +0,0 @@\n-line"},
	}
	scope := BuildDiff(changes, nil, 96*1024)
	if !scope.Formatting["a.go"].Violating() || scope.Formatting["a.go"].FormattingHunks != 1 {
		t.Fatalf("a.go not flagged: %+v", scope.Formatting["a.go"])
	}
	if scope.Formatting["b.go"].Violating() || scope.Formatting["d.go"].Violating() {
		t.Fatal("added/deleted file flagged")
	}
	if !scope.Formatting["c.go"].Violating() {
		t.Fatal("c.go not flagged")
	}
	stat := scope.Formatting["a.go"]
	if len(stat.Examples) != 1 || !scope.Added["a.go"][stat.Examples[0].HeadLine] {
		t.Fatalf("example anchor is not an included added line: %+v %+v", stat.Examples, scope.Added["a.go"])
	}
	// Chunked diffs of one path accumulate instead of overwriting.
	chunked := BuildDiff([]Change{
		{NewPath: "e.go", OldPath: "e.go", Diff: "@@ -1,1 +1,1 @@\n-a\n+  a"},
		{NewPath: "e.go", OldPath: "e.go", Diff: "@@ -5,1 +5,1 @@\n-b\n+  b"},
	}, nil, 96*1024)
	if chunked.Formatting["e.go"].Hunks != 2 || chunked.Formatting["e.go"].FormattingHunks != 2 || len(chunked.Formatting["e.go"].Examples) != 2 {
		t.Fatalf("chunked statistics not merged: %+v", chunked.Formatting["e.go"])
	}
	merged := DiffScope{Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Metadata: map[string]GitChangeMetadata{}}
	mergeScopeAnchors(&merged, scope)
	mergeScopeAnchors(&merged, chunked)
	if !merged.Formatting["a.go"].Violating() || merged.Formatting["e.go"].FormattingHunks != 2 {
		t.Fatalf("scope merge lost formatting statistics: %+v", merged.Formatting)
	}
}

func TestFormattingScopeFindingsInjected(t *testing.T) {
	scope := BuildDiff([]Change{
		{NewPath: "a.go", OldPath: "a.go", Diff: "@@ -1,3 +1,3 @@\n func main() {\n-    run()\n+        run()\n }"},
		{NewPath: "b.go", OldPath: "b.go", Added: true, Diff: "@@ -0,0 +1,1 @@\n+safe()"},
	}, nil, 96*1024)
	repo := fixtureRepo{files: map[string]string{"a.go": "func main() {\n        run()\n}", "b.go": "safe()"}}
	tools := &auditTools{repo: repo, snap: Snapshot{ProjectID: 1, MRIID: 2, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, scope: scope, cache: map[string]string{}, trace: []ToolTrace{}}
	if notes := tools.recordFormattingScopeFindings(context.Background()); len(notes) != 0 {
		t.Fatalf("unexpected notes: %v", notes)
	}
	findings := tools.acceptedFindings()
	if len(findings) != 1 {
		t.Fatalf("expected one deterministic finding, got %+v", findings)
	}
	f := findings[0]
	if f.Origin != deterministicFormattingOrigin || f.Type != "formatting scope" || f.Severity != "low" || f.Confidence != "candidate" || f.Side != "head" {
		t.Fatalf("unexpected finding fields: %+v", f)
	}
	if f.File != "a.go" || f.Line != 2 || f.Evidence != "        run()" || f.ID == "" || f.Fingerprint == "" {
		t.Fatalf("unexpected anchor: %+v", f)
	}
	if f.Verification != nil || f.SequenceDiagram != nil || f.PRContext != nil || f.InvestigationID != "" {
		t.Fatalf("deterministic finding carries model-phase state: %+v", f)
	}
	if notes := findingPRCoverage(findings); len(notes) != 0 {
		t.Fatalf("deterministic finding must not add PR recording gap notes: %v", notes)
	}
	// Idempotent: identical input keeps a single finding under the same ID.
	tools.recordFormattingScopeFindings(context.Background())
	again := tools.acceptedFindings()
	if len(again) != 1 || again[0].ID != f.ID {
		t.Fatalf("deterministic findings are not stable: %+v", again)
	}
}

func TestFormattingScopeFindingsBoundedAndDisclosed(t *testing.T) {
	changes := []Change{}
	repo := fixtureRepo{files: map[string]string{}}
	for i := 0; i <= maxFormattingScopeFindings; i++ {
		p := fmt.Sprintf("f%02d.go", i)
		changes = append(changes, Change{NewPath: p, OldPath: p, Diff: "@@ -1,1 +1,1 @@\n-a\n+ a"})
		repo.files[p] = " a\n"
	}
	changes = append(changes, Change{NewPath: "blank.go", OldPath: "blank.go", Diff: "@@ -1,1 +1,1 @@\n-  \n+"})
	repo.files["blank.go"] = ""
	scope := BuildDiff(changes, nil, 96*1024)
	tools := &auditTools{repo: repo, snap: Snapshot{HeadSHA: "h"}, scope: scope, cache: map[string]string{}, trace: []ToolTrace{}}
	notes := tools.recordFormattingScopeFindings(context.Background())
	if got := len(tools.acceptedFindings()); got != maxFormattingScopeFindings {
		t.Fatalf("cap not enforced: %d", got)
	}
	joined := strings.Join(notes, ";")
	if !strings.Contains(joined, "truncated") || !strings.Contains(joined, "blank.go") {
		t.Fatalf("truncation or skip not disclosed: %v", notes)
	}
}

func TestFormattingScopeSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Snapshot().CheckFormattingScope {
		t.Fatal("check_formatting_scope must default to false")
	}
	current := svc.Snapshot()
	current.CheckFormattingScope = true
	if err = svc.Save(current); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil || !reopened.Snapshot().CheckFormattingScope {
		t.Fatal("setting not durable", err)
	}
	// A settings save that omits the field keeps the configured value.
	decoded, err := reopened.DecodePublic([]byte(`{"verify_findings":true}`))
	if err != nil || !decoded.CheckFormattingScope {
		t.Fatal("omitted field reset the setting", err)
	}
	decoded, err = reopened.DecodePublic([]byte(`{"check_formatting_scope":false}`))
	if err != nil || decoded.CheckFormattingScope {
		t.Fatal("explicit field ignored", err)
	}
	if policy := capturePolicy(Settings{CheckFormattingScope: true}); !policy.CheckFormattingScope {
		t.Fatal("policy did not capture the setting")
	}
}
