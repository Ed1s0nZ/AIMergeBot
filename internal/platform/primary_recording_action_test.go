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

func TestRecordingNextActionPrioritizesActualUnfinishedState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state primaryRecordingProgress
		want  string
	}{
		{"no_source", primaryRecordingProgress{}, "inspect_changed_source"},
		{"source_no_ledger", primaryRecordingProgress{HeadSourceIDs: []string{"observation-2"}, UninspectedContextIDs: []int{2}}, "record_changed_behavior"},
		{"missing_plan", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"pr_context_missing", "plan_missing"}}, "repair_plan"},
		{"unfinished_plan", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"plan_unfinished", "head_sources_missing"}}, "inspect_plan"},
		{"context_missing", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"pr_context_missing"}}, "link_pr_context"},
		{"side_missing", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"base_sources_missing"}}, "link_pr_sides"},
		{"unread_context", primaryRecordingProgress{LedgerCount: 1, UninspectedContextIDs: []int{2}, RecordingGaps: []string{"relationships_missing"}}, "inspect_context"},
		{"relation_missing", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"relationships_missing"}, UnresolvedLedgerCount: 1}, "record_relationships"},
		{"not_resolved", primaryRecordingProgress{LedgerCount: 1, UnresolvedLedgerCount: 1}, "resolve_hypotheses"},
		{"pending_correction", primaryRecordingProgress{LedgerCount: 1, EligibleRecordingCorrections: []recordingCorrection{{}}}, "resolve_recording_errors"},
		{"inferred_stays_unknown", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"relationships_inferred"}}, "summarize_with_limits"},
		{"unknown_gap", primaryRecordingProgress{LedgerCount: 1, RecordingGaps: []string{"future_gap"}}, "summarize_with_limits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := nextRecordingAction(tc.state)
			if got != tc.want {
				t.Fatal(got, tc.want)
			}
			tc.state.NextRecordingAction = got
			nav := recordingProgressGuidance(tc.state)
			if !strings.Contains(nav, `"next_recording_action":"`+got+`"`) || !strings.Contains(nav, "final-decision strict JSON instruction takes precedence") {
				t.Fatal(nav)
			}
			if tc.name == "inferred_stays_unknown" && !strings.Contains(nav, "semantic completeness") {
				t.Fatal("inferred state implied completion")
			}
		})
	}
	empty := primaryRecordingProgress{NextRecordingAction: "inspect_changed_source"}
	nav := recordingProgressGuidance(empty)
	if strings.Contains(nav, "same-statement recording pairs") || strings.Contains(nav, "Uninspected configured context") {
		t.Fatal("irrelevant repeated guidance", nav)
	}
}

func TestRecordingNextActionUpdatesThroughRealSDKWithoutInventingPlanEvidence(t *testing.T) {
	repo, snap, _, _ := sequenceFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var request struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		system := ""
		for _, msg := range request.Messages {
			if msg.Role == "system" {
				system += msg.Content
			}
		}
		want := map[int32]string{1: "inspect_changed_source", 2: "record_changed_behavior", 3: "inspect_plan"}[n]
		if !strings.Contains(system, `"next_recording_action":"`+want+`"`) {
			t.Error("missing actual next action", n, system)
		}
		if strings.Contains(system, "PRIVATE CLAIM") || strings.Contains(system, "PRIVATE QUESTION") || strings.Contains(system, "service.any") {
			t.Error("model/path text in trusted navigation")
		}
		message := map[string]any{"role": "assistant"}
		finish := "tool_calls"
		name, args := "read_file", `{"path":"service.any","start":1,"end":3}`
		if n == 2 {
			inv := Investigation{ID: "inspection", Claim: "PRIVATE CLAIM", ObservationIDs: []string{"observation-2"}}
			for i, kind := range verificationKinds {
				inv.Plan = append(inv.Plan, InvestigationTask{ID: []string{"input", "change", "guard", "effect"}[i], Kind: kind, Question: "PRIVATE QUESTION", Status: "pending"})
			}
			raw, _ := json.Marshal(inv)
			name, args = "record_hypothesis", string(raw)
		}
		if n < 3 {
			message["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}
		} else {
			finish = "stop"
			message["content"] = `{"findings":[],"summary":"inspection remains unfinished","coverage_notes":[]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 3, MaxToolCalls: 4}}
	result, trace, err := auditor.Audit(context.Background(), snap, DiffScope{})
	if err != nil || calls.Load() != 3 || len(result.Investigations) != 1 || len(result.CoverageNotes) == 0 {
		t.Fatal(err, calls.Load(), result)
	}
	inv := result.Investigations[0]
	if inv.Status != "investigating" || inv.PRContext != nil {
		t.Fatal("navigation invented resolved context", inv)
	}
	for _, task := range inv.Plan {
		if task.Status != "pending" || len(task.ObservationIDs) != 0 {
			t.Fatal("navigation invented checked source", task)
		}
	}
	if len(trace) < 3 || trace[0].Name != "list_files" {
		t.Fatal("ordinary preflight provenance lost", trace)
	}
}
