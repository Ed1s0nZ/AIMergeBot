package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGroupedSDKCancellationCheckpointsNamespacedClaimReview(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		name, args := "read_file", `{"path":"a.go","base":true,"start":1,"end":1}`
		if n == 2 {
			args = `{"path":"a.go","base":false,"start":1,"end":1}`
		}
		if n >= 3 {
			ids := []string{}
			for _, m := range req.Messages {
				if m.Role == "tool" {
					var out toolOutput
					if json.Unmarshal([]byte(m.Content), &out) == nil && out.EvidenceEligible {
						ids = append(ids, out.ObservationID)
					}
				}
			}
			if len(ids) != 2 {
				t.Errorf("expected BASE/HEAD source responses, got %v", ids)
				w.WriteHeader(400)
				return
			}
			inv := Investigation{ID: "local", Claim: "old becomes new", ObservationIDs: ids, Evidence: []string{"new"}, PRContext: &PRInvestigationContext{ChangeSummary: "old to new", Before: "old", After: "new", BeforeObservationIDs: ids[:1], AfterObservationIDs: ids[1:2], Relationships: []InvestigationRelationship{{From: "BASE", To: "HEAD", Relation: "same path", Certainty: "cited", ObservationIDs: ids}}}}
			for i, kind := range verificationKinds {
				inv.Plan = append(inv.Plan, InvestigationTask{ID: []string{"input", "change", "guard", "effect"}[i], Kind: kind, Question: "Inspect source", Status: "checked", Reason: "source inspected", ObservationIDs: ids})
			}
			name = "record_hypothesis"
			if n == 4 {
				name = "update_investigation"
				inv.Status = "supported"
			}
			raw, _ := json.Marshal(inv)
			args = string(raw)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}}}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
	}))
	defer server.Close()
	scope := DiffScope{Text: "@@ -1 +1 @@\n-old\n+new", Included: []string{"a.go"}, Added: map[string]map[int]bool{"a.go": {1: true}}, Removed: map[string]map[int]bool{"a.go": {1: true}}}
	auditor := EinoAuditor{Repository: coverageComparisonRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 5, MaxToolCalls: 20, VerifyFindings: true, Progress: func(result AuditResult, trace []ToolTrace) error {
		if err := s.Checkpoint(ctx, id, result, trace); err != nil {
			return err
		}
		if len(result.Investigations) == 1 && result.Investigations[0].Status == "supported" {
			if err := s.Cancel(ctx, id, 1); err != nil {
				return err
			}
			return ErrConflict
		}
		return nil
	}}}
	_, _, err = auditor.AuditGroups(ctx, snap, AuditPlan{Groups: []AuditGroup{{ID: "g1", Files: []string{"a.go"}, Scope: scope}, {ID: "g2", Files: []string{"a.go"}, Scope: scope}}})
	if !errors.Is(err, ErrConflict) || calls.Load() != 4 {
		t.Fatal("cancel fence failed", err, calls.Load())
	}
	run, err := s.Run(ctx, id)
	if err != nil || run.Status != "cancelled" || len(run.Result.Investigations) != 1 {
		t.Fatal(err, run)
	}
	item := run.Result.Investigations[0]
	if item.ID != "g1-local" || item.Status != "supported" || item.ClaimVerification == nil || item.ClaimVerification.Status != "unavailable" || item.ClaimVerification.Verdict != "" {
		t.Fatal("group checkpoint review lost", item)
	}
	notes := strings.Join(run.Result.CoverageNotes, "\n")
	if !strings.Contains(notes, "review unavailable: g1-local") || strings.Contains(notes, "review unavailable: local") {
		t.Fatal("review projected before group identity", notes)
	}
}
