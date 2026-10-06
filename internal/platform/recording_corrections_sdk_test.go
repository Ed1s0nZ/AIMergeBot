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

func TestRecordingCorrectionSDKRetiresOnlyCorrectedWork(t *testing.T) {
	repo, snap, _, _ := sequenceFixture()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		message := map[string]any{"role": "assistant"}
		finish := "tool_calls"
		name := ""
		var args any
		switch n {
		case 1:
			name = "read_file"
			args = readArgs{Path: "service.any", Start: 1, End: 2}
		case 2:
			name = "record_hypothesis"
			args = Investigation{ID: "inv", Claim: "conditional candidate", ObservationIDs: []string{"unknown"}}
		case 3:
			name = "record_hypothesis"
			args = Investigation{ID: "inv", Claim: "conditional candidate", ObservationIDs: []string{"observation-1"}}
		case 4:
			name = "resolve_recording_errors"
			args = recordingCorrectionsArgs{Corrections: []recordingCorrection{{"observation-2", "observation-3"}}}
		default:
			finish = "stop"
			message["content"] = `{"findings":[],"summary":"fixture retains unfinished plan","coverage_notes":[]}`
		}
		if name != "" {
			raw, _ := json.Marshal(args)
			message["tool_calls"] = []any{map[string]any{"id": name, "type": "function", "function": map[string]string{"name": name, "arguments": string(raw)}}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}, "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
	}))
	defer server.Close()
	a := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 6, MaxToolCalls: 8}}
	result, trace, err := a.Audit(context.Background(), snap, DiffScope{})
	if err != nil || calls.Load() != 5 {
		t.Fatal("SDK flow failed", calls.Load(), err)
	}
	corrected := false
	retained := false
	for _, tr := range trace {
		retained = retained || tr.Name == "record_hypothesis" && tr.Error != ""
		if tr.Name == "resolve_recording_errors" {
			var output toolOutput
			json.Unmarshal([]byte(tr.Output), &output)
			corrected = tr.Error == "" && output.Error == "" && !output.EvidenceEligible
		}
	}
	if !corrected || !retained {
		t.Fatal("correction not traced or history erased", trace)
	}
	planGap := false
	unresolved := false
	for _, note := range result.CoverageNotes {
		if strings.Contains(note, "Tool failed: record_hypothesis") {
			t.Fatal("corrected record still pending", result.CoverageNotes)
		}
		planGap = planGap || strings.Contains(note, "plan recording gap")
		unresolved = unresolved || strings.Contains(note, "Unresolved hypothesis")
	}
	if !planGap || !unresolved || len(result.Findings) != 0 {
		t.Fatal("actual incompleteness removed", result)
	}
}
