package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sequenceFixture() (fixtureRepo, Snapshot, Finding, SequenceInput) {
	repo := fixtureRepo{files: map[string]string{"service.any": "request(input)\ndanger(input)\n", "caller.any": "service(request)\n"}}
	snap := Snapshot{BaseSHA: "base-commit", HeadSHA: "head-commit"}
	f := Finding{Side: "head", Type: "unsafe call", File: "service.any", Line: 2, Severity: "high", Title: "unsafe sink", Description: "untrusted input reaches sink", Evidence: "danger(input)", Trigger: "external input", Suggestion: "validate input", Confidence: "candidate"}
	input := SequenceInput{Participants: []SequenceParticipant{{ID: "caller", Label: "请求入口"}, {ID: "service", Label: "资源服务"}, {ID: "sink", Label: "危险操作"}}, Steps: []SequenceStep{{From: "caller", To: "service", Label: "外部输入进入服务", Kind: "call", Certainty: "inferred", Evidence: []SequenceReference{}}, {From: "service", To: "sink", Label: "执行危险操作，缺少输入校验", Kind: "call", Certainty: "cited", Risk: true, Evidence: []SequenceReference{{Side: "head", File: f.File, Line: f.Line, Snippet: f.Evidence}}}}, Limitations: []string{"入口到服务的关联尚未确认；未运行复现"}}
	return repo, snap, f, input
}
func TestSequenceValidationAndSafeMermaid(t *testing.T) {
	repo, snap, f, input := sequenceFixture()
	graph, e := ValidateSequence(context.Background(), repo, snap, f, input)
	if e != nil {
		t.Fatal(e)
	}
	if graph.Status != "partial" || graph.Steps[1].Evidence[0].SHA != snap.HeadSHA || !strings.Contains(graph.Mermaid, "rect rgb(255, 237, 237)") || !strings.Contains(graph.Mermaid, "caller-->>service") {
		t.Fatal(graph)
	}
	raw, _ := json.Marshal(input)
	cases := map[string]func(*SequenceInput){"fake snippet": func(g *SequenceInput) { g.Steps[1].Evidence[0].Snippet = "invented" }, "wrong sha": func(g *SequenceInput) { g.Steps[1].Evidence[0].SHA = "wrong" }, "outside path": func(g *SequenceInput) { g.Steps[1].Evidence[0].File = "../config.yaml" }, "fake line": func(g *SequenceInput) { g.Steps[1].Evidence[0].Line = 99 }, "unknown participant": func(g *SequenceInput) { g.Steps[0].To = "missing" }, "duplicate participant": func(g *SequenceInput) { g.Participants[1].ID = "caller" }, "participant directive": func(g *SequenceInput) { g.Participants[0].ID = "a\n%%{init}" }, "multiline label": func(g *SequenceInput) { g.Steps[0].Label = "hello\nend" }, "no risk anchor": func(g *SequenceInput) { g.Steps[1].Risk = false }, "missing cited refs": func(g *SequenceInput) { g.Steps[1].Evidence = nil }, "base execution": func(g *SequenceInput) { g.Steps[1].Evidence[0].Side = "base" }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var altered SequenceInput
			json.Unmarshal(raw, &altered)
			mutate(&altered)
			if _, e := ValidateSequence(context.Background(), repo, snap, f, altered); e == nil {
				t.Fatal("invalid sequence accepted")
			}
		})
	}
	input.Participants[0].Label = "<img src=x onerror=alert(1)>"
	input.Steps[0].Label = "%%{init: unsafe}; <script>"
	graph, e = ValidateSequence(context.Background(), repo, snap, f, input)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(graph.Mermaid, "<img") || strings.Contains(graph.Mermaid, "<script") || strings.Contains(graph.Mermaid, "%%{") {
		t.Fatal("unescaped Mermaid label")
	}
	for _, raw := range []string{`{}`, `{"participants":[],"steps":[],"limitations":[],"mermaid":"unsafe"}`, `{} {}`, "```json\n{}\n```"} {
		input, e := ParseSequence(raw)
		if e == nil {
			if _, e = ValidateSequence(context.Background(), repo, snap, f, input); e == nil {
				t.Fatal("bad graph accepted")
			}
		}
	}
}
func TestSequenceBaseNoteAndModelDiagramIsUntrusted(t *testing.T) {
	repo, snap, f, input := sequenceFixture()
	f.Side = "base"
	input.Steps[1].Kind = "note"
	input.Steps[1].Label = "变更删除了检查"
	input.Steps[1].Evidence[0].Side = "base"
	graph, e := ValidateSequence(context.Background(), repo, snap, f, input)
	if e != nil || !strings.Contains(graph.Mermaid, "变更前") {
		t.Fatal(graph, e)
	}
	f.Side = "head"
	f.SequenceDiagram = &SequenceDiagram{Status: "ready", Mermaid: "malicious"}
	result := AuditResult{Findings: []Finding{f}}
	scope := DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}}
	if e = ValidateFindings(context.Background(), repo, snap, scope, &result); e != nil || result.Findings[0].SequenceDiagram != nil {
		t.Fatal("primary model diagram trusted", e)
	}
}
func TestEinoConditionalSequenceGenerationPreservesFindings(t *testing.T) {
	for _, mode := range []string{"valid", "invalid", "http_failure", "clean", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, f, input := sequenceFixture()
			var auditCalls, graphCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
					t.Error(e)
				}
				diagram := len(req.Messages) > 0 && strings.HasPrefix(req.Messages[0].Content, "Generate a static sequence diagram")
				var content string
				if diagram {
					graphCalls.Add(1)
					assertRequiredToolCapabilities(t, req.Tools, "read_file", "get_diff")
					for _, v := range req.Tools {
						if v.Function.Name == "submit_finding" || v.Function.Name == "record_hypothesis" || v.Function.Name == "update_investigation" || v.Function.Name == "resolve_recording_errors" {
							t.Error("process write tool exposed to diagram phase")
						}
					}
					if mode == "http_failure" {
						w.WriteHeader(400)
						fmt.Fprint(w, `{"error":{"message":"fixture diagram failure","type":"invalid_request_error"}}`)
						return
					}
					if mode == "invalid" {
						input.Steps[1].Evidence[0].Snippet = "invented"
					}
					raw, _ := json.Marshal(input)
					content = string(raw)
				} else {
					auditCalls.Add(1)
					findings := []Finding{f}
					if mode == "clean" {
						findings = []Finding{}
					}
					raw, _ := json.Marshal(AuditResult{Findings: findings, Summary: "Fixture finding, not runtime verified", CoverageNotes: []string{}})
					content = string(raw)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50}, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
			}))
			defer server.Close()
			scope := DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}, Text: "untrusted diff"}
			auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "fixture", BaseURL: server.URL, Model: "fixture", MaxSteps: 8, GenerateDiagrams: mode != "disabled"}}
			result, trace, e := auditor.Audit(context.Background(), snap, scope)
			if e != nil {
				t.Fatal("diagram error discarded audit", e)
			}
			if auditCalls.Load() != 1 {
				t.Fatal("unexpected primary audit calls")
			}
			if mode == "clean" {
				if len(result.Findings) != 0 || graphCalls.Load() != 0 {
					t.Fatal("clean audit generated diagrams")
				}
				return
			}
			if len(result.Findings) != 1 || result.Findings[0].ID == "" {
				t.Fatal("lost primary finding")
			}
			graph := result.Findings[0].SequenceDiagram
			if mode == "valid" {
				if graph == nil || graph.Status != "partial" || graph.Mermaid == "" {
					t.Fatal(graph)
				}
				foundUsage := false
				for _, tr := range trace {
					if tr.Stage == "diagram" && tr.Name == "model" && tr.UsageReported {
						foundUsage = true
					}
				}
				if !foundUsage {
					t.Fatal("diagram token usage missing")
				}
				if proof := os.Getenv("AIM_SEQUENCE_FIXTURE_PROOF"); proof != "" {
					raw, _ := json.Marshal(Run{Snapshot: snap, Status: "succeeded", Result: result, Trace: trace, PolicyVersion: PolicyVersion})
					if e = os.WriteFile(proof, raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
			} else if graph == nil || graph.Status != "unavailable" || graph.Reason == "" {
				t.Fatal("failure or disabled state missing", graph)
			}
			if mode == "disabled" && graphCalls.Load() != 0 {
				t.Fatal("disabled generated diagrams")
			}
			if mode != "disabled" && graphCalls.Load() != 1 {
				t.Fatal("unexpected graph calls")
			}
			if len(result.CoverageNotes) != 2 || !hasPlanGap(result.CoverageNotes) || !strings.Contains(result.CoverageNotes[1], "PR impact recording gap") {
				t.Fatal("supplemental diagram changed primary chain coverage")
			}
		})
	}
}
func TestSequenceSettingsPersistAndLegacyDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, e := OpenSettings(filepath.Join(dir, "config.yaml"), "../../config.example.yaml")
	if e != nil {
		t.Fatal(e)
	}
	if !cfg.Snapshot().GenerateSequenceDiagrams {
		t.Fatal("default generation disabled")
	}
	next := cfg.Snapshot()
	next.GenerateSequenceDiagrams = false
	if e = cfg.Save(next); e != nil {
		t.Fatal(e)
	}
	again, e := OpenSettings(cfg.path, "../../config.example.yaml")
	if e != nil || again.Snapshot().GenerateSequenceDiagrams {
		t.Fatal("disabled setting did not persist")
	}
	raw, _ := os.ReadFile(cfg.path)
	raw = []byte(strings.ReplaceAll(string(raw), "generate_sequence_diagrams: false\n", ""))
	os.WriteFile(cfg.path, raw, 0600)
	again, e = OpenSettings(cfg.path, "../../config.example.yaml")
	if e != nil || !again.Snapshot().GenerateSequenceDiagrams {
		t.Fatal("legacy config default missing")
	}
}
