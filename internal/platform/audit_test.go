package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureRepo struct{ files map[string]string }

func (f fixtureRepo) Snapshot(context.Context, int, int) (Snapshot, error) { return Snapshot{}, nil }
func (f fixtureRepo) Changes(context.Context, Snapshot) ([]Change, []string, error) {
	return nil, nil, nil
}
func (f fixtureRepo) ReadFile(_ context.Context, _ Snapshot, p string, _ bool) (string, error) {
	s, ok := f.files[p]
	if !ok {
		return "", fmt.Errorf("missing")
	}
	return s, nil
}
func (f fixtureRepo) ListFiles(context.Context, Snapshot, int) ([]string, bool, error) {
	return []string{}, false, nil
}

func TestDiffAndEvidence(t *testing.T) {
	scope := BuildDiff([]Change{{NewPath: "a.go", OldPath: "a.go", Diff: "@@ -1,2 +1,3 @@\n safe\n-old\n+danger(input)\n+guard()"}}, nil, 10000)
	if !scope.Added["a.go"][2] || !scope.Added["a.go"][3] || scope.Added["a.go"][1] {
		t.Fatal("bad hunk line mapping")
	}
	repo := fixtureRepo{files: map[string]string{"a.go": "safe\ndanger(input)\nguard()"}}
	finding := Finding{Type: "unsafe sink", File: "a.go", Line: 2, Severity: "high", Title: "unsafe sink", Description: "input reaches sink", Evidence: "danger(input)", Trigger: "external input", Suggestion: "validate", Confidence: "candidate"}
	result := AuditResult{Findings: []Finding{finding, finding}}
	if err := ValidateFindings(context.Background(), repo, Snapshot{HeadSHA: "h"}, scope, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID == "" {
		t.Fatal("dedup failed")
	}
	finding.Evidence = "invented"
	result.Findings = []Finding{finding}
	if ValidateFindings(context.Background(), repo, Snapshot{}, scope, &result) == nil {
		t.Fatal("invented evidence accepted")
	}
	finding.Line = 1
	result.Findings = []Finding{finding}
	if ValidateFindings(context.Background(), repo, Snapshot{}, scope, &result) == nil {
		t.Fatal("unchanged line accepted")
	}
}
func TestStrictResultDoesNotInventFindings(t *testing.T) {
	for _, bad := range []string{"", `{}`, `{"findings":[],"summary":"ok","coverage_notes":[],"unknown":1}`, "```json\n{}\n```", `{"findings":[],"summary":"ok","coverage_notes":[]} {}`} {
		if _, err := ParseResult(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	result, err := ParseResult(`{"findings":[],"summary":"No issue supported by available evidence","coverage_notes":[]}`)
	if err != nil || len(result.Findings) != 0 {
		t.Fatal("clean response fabricated findings")
	}
}
func TestSettingsPersistenceAndRedaction(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "config.example.yaml")
	target := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(example, data, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := OpenSettings(target, example)
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.OpenAI.APIKey = "test-secret-key"
	cfg.GitLab.Token = "test-gitlab-secret"
	cfg.OpenAI.Model = "test-model"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(raw), "test-model") {
		t.Fatal("settings not persisted")
	}
	public := svc.Public()
	if public["has_api_key"] != true {
		t.Fatal("missing secret indicator")
	}
	if public["openai"].(map[string]interface{})["api_key"] != "" {
		t.Fatal("secret exposed")
	}
	cfg = svc.Snapshot()
	cfg.OpenAI.APIKey = ""
	cfg.GitLab.Token = ""
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if svc.Snapshot().OpenAI.APIKey != "test-secret-key" {
		t.Fatal("blank erased secret")
	}
	before := svc.Snapshot()
	cfg.AuditWorkers = 0
	if svc.Save(cfg) == nil {
		t.Fatal("invalid setting saved")
	}
	if svc.Snapshot().AuditWorkers != before.AuditWorkers {
		t.Fatal("failed save changed live config")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0600 {
		t.Fatal("config permissions expose secrets")
	}
}

func TestProjectConfigurationSync(t *testing.T) {
	dir := t.TempDir()
	svc, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := svc.Snapshot()
	cfg.OpenAI.Model = "preserved-model"
	if err = svc.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err = svc.SyncProjects([]Project{{ID: 7, Name: "group/fork", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reloaded.Snapshot()
	if len(snapshot.Projects) != 1 || snapshot.Projects[0].Enabled == nil || *snapshot.Projects[0].Enabled || snapshot.OpenAI.Model != "preserved-model" {
		t.Fatal("project settings did not persist")
	}
	*snapshot.Projects[0].Enabled = true
	if *reloaded.Snapshot().Projects[0].Enabled {
		t.Fatal("snapshot can mutate live config")
	}
}

type removedRepo struct{ fixtureRepo }

func (removedRepo) ReadFile(_ context.Context, _ Snapshot, p string, base bool) (string, error) {
	if p == "auth.go" && base {
		return "authorize(user)\n", nil
	}
	return "", fmt.Errorf("deleted file absent at head")
}
func TestRemovedGuardBaseEvidence(t *testing.T) {
	scope := BuildDiff([]Change{{OldPath: "auth.go", NewPath: "auth.go", Deleted: true, Diff: "@@ -1 +0,0 @@\n-authorize(user)"}}, nil, 10000)
	f := Finding{Side: "base", Type: "authorization", File: "auth.go", Line: 1, Severity: "high", Title: "removed authorization guard", Description: "guard removed", Evidence: "authorize(user)", Trigger: "request proceeds after guard removal", Suggestion: "restore authorization check", Confidence: "candidate"}
	result := AuditResult{Findings: []Finding{f}}
	if err := ValidateFindings(context.Background(), removedRepo{}, Snapshot{HeadSHA: "head", BaseSHA: "base"}, scope, &result); err != nil {
		t.Fatal(err)
	}
	if result.Findings[0].Side != "base" {
		t.Fatal("base evidence relabeled as head")
	}
	f.Side = "head"
	result.Findings = []Finding{f}
	if ValidateFindings(context.Background(), removedRepo{}, Snapshot{}, scope, &result) == nil {
		t.Fatal("deleted evidence accepted as head")
	}
}
