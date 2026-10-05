package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGroupedAgentRetainsCompletedFindingOnLaterFailureOrBudget(t *testing.T) {
	for _, mode := range []string{"failure", "budget"} {
		t.Run(mode, func(t *testing.T) {
			repo, snap, f, _ := sequenceFixture()
			f.ObservationIDs = []string{"group-1-observation-1"}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				n := calls.Add(1)
				if n == 3 {
					if len(request.Messages) < 2 || !strings.Contains(request.Messages[1].Content, "Current audit group") || !strings.Contains(request.Messages[1].Content, "do not resubmit") || !strings.Contains(request.Messages[1].Content, `"id":"group-2"`) || !strings.Contains(request.Messages[1].Content, "Prior group navigation") || !strings.Contains(request.Messages[1].Content, "unsafe sink") || !strings.Contains(request.Messages[1].Content, "source_locators") || !strings.Contains(request.Messages[1].Content, "group-1-observation-1") {
						t.Error("later group lost source handoff")
					}
					w.WriteHeader(400)
					return
				}
				if n == 1 && !strings.Contains(request.Messages[1].Content, "Changed-path manifest") {
					t.Error("cross-group manifest missing")
				}
				message := map[string]any{"role": "assistant"}
				finish := "stop"
				if n == 1 {
					message["tool_calls"] = []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"service.any","start":2,"end":2}`}}}
					finish = "tool_calls"
				} else {
					raw, _ := json.Marshal(AuditResult{Findings: []Finding{f}, Summary: "first group retained", CoverageNotes: []string{}})
					message["content"] = string(raw)
				}
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
			}))
			defer server.Close()
			budget := 80
			if mode == "budget" {
				budget = 1
			}
			checkpoints := []AuditResult{}
			auditor := &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "group-fixture", MaxSteps: 8, MaxToolCalls: budget, Progress: func(result AuditResult, _ []ToolTrace) error { checkpoints = append(checkpoints, result); return nil }}}
			scope := DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}, Text: "File: service.any\n@@ -2 +2 @@\n+danger(input)"}
			plan := AuditPlan{Groups: []AuditGroup{{ID: "group-1", Files: []string{f.File}, Scope: scope}, {ID: "group-2", Files: []string{"other.any"}, Scope: scope}}}
			result, trace, err := auditor.AuditGroups(context.Background(), snap, plan)
			if err != nil || len(result.Findings) != 1 || len(result.CoverageNotes) == 0 || result.AuditGroups[0].Status != "completed" {
				t.Fatal("completed discovery lost", result, err)
			}
			want := "failed"
			if mode == "budget" {
				want = "unprocessed"
			}
			if result.AuditGroups[1].Status != want {
				t.Fatal("incomplete group falsely completed", result.AuditGroups)
			}
			found := false
			for _, tr := range trace {
				if tr.Name == "read_file" {
					found = strings.HasPrefix(tr.ObservationID, "group-1-observation-")
				}
			}
			if !found || len(checkpoints) == 0 || len(checkpoints[len(checkpoints)-1].Findings) != 1 {
				t.Fatal("group provenance/checkpoint missing")
			}
		})
	}
}

func TestGroupMergeNamespacesInvestigationsWithoutMutatingSource(t *testing.T) {
	source := AuditResult{Findings: []Finding{{ID: "f", InvestigationID: "hypothesis-1"}}, Investigations: []Investigation{{ID: "hypothesis-1"}}}
	merged := mergeAuditGroup(AuditResult{}, source, "group-2")
	if source.Findings[0].InvestigationID != "hypothesis-1" || merged.Findings[0].InvestigationID != "group-2-hypothesis-1" || merged.Investigations[0].ID != "group-2-hypothesis-1" {
		t.Fatal("investigation reference collision or mutation", source, merged)
	}
}

func TestSynthesisParserCannotRewriteCanonicalFindings(t *testing.T) {
	good := `{"summary":"Static grouped review","coverage_notes":[]}`
	if _, err := parseGroupSynthesis(good); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{good + `{}`, `{"summary":"ok","coverage_notes":[],"findings":[]}`, `{"summary":"","coverage_notes":[]}`, `{"summary":"ok","coverage_notes":null}`} {
		if _, err := parseGroupSynthesis(raw); err == nil {
			t.Fatal("unsafe synthesis accepted", raw)
		}
	}
}

func TestGroupedSynthesisKeepsCanonicalFindingSet(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		n := calls.Add(1)
		content := ""
		if strings.HasPrefix(request.Messages[0].Content, "Summarize a grouped") {
			var payload struct {
				CoverageNotes []string `json:"coverage_notes"`
			}
			if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil || len(payload.CoverageNotes) == 0 {
				t.Error("synthesis lost coverage input", err)
			}
			notes := append(append([]string{}, payload.CoverageNotes...), payload.CoverageNotes...)
			notes = append(notes, "Additional fixture coverage", "Additional fixture coverage")
			raw, _ := json.Marshal(groupSynthesis{Summary: "Cross-group static summary, not reproduction", CoverageNotes: notes})
			content = string(raw)
		} else {
			findings := []Finding{}
			if n == 1 {
				findings = append(findings, f)
			}
			raw, _ := json.Marshal(AuditResult{Findings: findings, Summary: "group summary", CoverageNotes: []string{}})
			content = string(raw)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": content}}}})
	}))
	defer server.Close()
	auditor := &EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "group-fixture", MaxSteps: 8}}
	scope := DiffScope{Added: map[string]map[int]bool{f.File: {2: true}}, Text: "File: service.any\n@@ -2 +2 @@\n+danger(input)"}
	plan := AuditPlan{Groups: []AuditGroup{{ID: "group-1", Files: []string{f.File}, Scope: scope}, {ID: "group-2", Files: []string{"other.any"}, Scope: scope}}}
	result, trace, err := auditor.AuditGroups(context.Background(), snap, plan)
	if err != nil || len(result.Findings) != 1 || result.Findings[0].Confidence != "candidate" || result.Summary != "Cross-group static summary, not reproduction" || len(result.CoverageNotes) != 2 || !strings.Contains(result.CoverageNotes[0], "PR impact recording gap") || result.CoverageNotes[1] != "Additional fixture coverage" || calls.Load() != 3 {
		t.Fatal("synthesis overwrote canonical discovery or missing", result, err, calls.Load())
	}
	found := false
	for _, tr := range trace {
		if tr.Name == "model" && tr.Stage == "synthesis" {
			found = true
		}
	}
	if !found {
		t.Fatal("synthesis model usage stage absent")
	}
}
