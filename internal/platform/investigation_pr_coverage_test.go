package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInvestigationPRCoverageDoesNotHideNoFindingGaps(t *testing.T) {
	tools := &auditTools{}
	for _, status := range []string{"supported", "rejected", "investigating"} {
		item := Investigation{ID: "inspection", Status: status, Claim: "unchanged fixture claim", PRContext: &PRInvestigationContext{Before: "before", After: "after"}}
		before, _ := json.Marshal(item)
		notes := tools.investigationPRCoverage([]Investigation{item})
		if len(notes) != 3 || !strings.Contains(strings.Join(notes, " "), "inspection") {
			t.Fatal(status, notes)
		}
		after, _ := json.Marshal(item)
		if string(before) != string(after) {
			t.Fatal("coverage changed original claims or links")
		}
		item.PRContext.BeforeObservationIDs = []string{"base"}
		item.PRContext.AfterObservationIDs = []string{"head"}
		item.PRContext.Relationships = []InvestigationRelationship{{Certainty: "cited"}}
		if len(tools.investigationPRCoverage([]Investigation{item})) != 0 {
			t.Fatal("populated fields flagged as missing")
		}
		item.PRContext.Relationships[0].Certainty = "inferred"
		notes = tools.investigationPRCoverage([]Investigation{item})
		if len(notes) != 1 || !strings.Contains(notes[0], "unverified") {
			t.Fatal("inferred edge hidden", notes)
		}
		item.PRContext = nil
		if len(tools.investigationPRCoverage([]Investigation{item})) != 1 {
			t.Fatal("absent comparison hidden")
		}
	}
	if len(tools.investigationPRCoverage(nil)) != 0 {
		t.Fatal("projection invented an investigation")
	}
}

func TestInvestigationMetadataCoverageRequiresActualIncludedCanonicalBasis(t *testing.T) {
	for _, mode := range []string{"valid", "no_ids", "wrong_id", "foreign", "stale", "verification", "error", "damaged", "duplicate", "changed_text", "changed_body", "mixed_source", "non_source"} {
		t.Run(mode, func(t *testing.T) {
			m := GitChangeMetadata{Kind: "modify", OldPath: "task.any", NewPath: "task.any", Base: &GitEntry{Mode: "100644", Type: "blob", ObjectID: strings.Repeat("a", 40)}, Head: &GitEntry{Mode: "100755", Type: "blob", ObjectID: strings.Repeat("a", 40)}}
			tools := &auditTools{snap: Snapshot{BaseSHA: strings.Repeat("b", 40), HeadSHA: strings.Repeat("c", 40)}, scope: DiffScope{Metadata: map[string]GitChangeMetadata{"task.any": m}}}
			out, err := tools.changeMetadata(context.Background(), metadataArgs{Path: "task.any"})
			if err != nil || out.Error != "" {
				t.Fatal(err, out)
			}
			item := Investigation{ID: "metadata", ObservationIDs: []string{out.ObservationID}}
			tr := &tools.trace[0]
			var saved toolOutput
			json.Unmarshal([]byte(tr.Output), &saved)
			switch mode {
			case "no_ids":
				item.ObservationIDs = nil
			case "wrong_id":
				saved.ObservationID = "wrong"
			case "foreign":
				saved.RepositoryID = 2
			case "stale":
				saved.HeadSHA = "stale"
			case "verification":
				tr.Stage = "verification"
			case "error":
				tr.Error = "fixture failure"
			case "damaged":
				tr.Output = "{"
			case "duplicate":
				tools.trace = append(tools.trace, *tr)
			case "changed_text":
				saved.Text += "not canonical"
			case "changed_body":
				m.Head.ObjectID = strings.Repeat("d", 40)
				tools.scope.Metadata["task.any"] = m
				saved.Metadata = &m
				saved.Text = m.canonical()
			case "mixed_source":
				item.CounterObservationIDs = []string{"body-source"}
			case "non_source":
				tr.Name = "list_files"
			}
			if mode != "damaged" && mode != "duplicate" {
				raw, _ := json.Marshal(saved)
				tr.Output = string(raw)
			}
			notes := tools.investigationPRCoverage([]Investigation{item})
			want := 1
			if mode == "valid" {
				want = 0
			}
			if len(notes) != want {
				t.Fatal("metadata exemption ignored its provenance", mode, notes)
			}
			if mode == "valid" {
				item.Claim = "mode changed"
				recorded, _ := tools.record(context.Background(), item)
				if recorded.Error != "" || strings.Contains(strings.Join(recorded.RecordingGaps, " "), "pr_context_missing") {
					t.Fatal("metadata tool feedback disagrees with final coverage", recorded)
				}
				if strings.Contains(tools.primaryProgressNavigation(), "pr_context_missing") {
					t.Fatal("live metadata projection disagrees with final coverage")
				}
			}
		})
	}
}

type coverageComparisonRepo struct{ runRepo }

func (coverageComparisonRepo) Changes(context.Context, Snapshot) ([]Change, []string, error) {
	return []Change{{OldPath: "a.go", NewPath: "a.go", Diff: "@@ -1 +1 @@\n-old\n+new"}}, nil, nil
}
func (coverageComparisonRepo) ReadFile(_ context.Context, _ Snapshot, _ string, base bool) (string, error) {
	if base {
		return "old", nil
	}
	return "new", nil
}

func TestNoFindingSDKInvestigationGapsPersistAsIncompleteWorkerResult(t *testing.T) {
	for _, mode := range []string{"supported", "rejected", "complete", "claim_disagreement", "claim_interrupted", "claim_cancelled"} {
		t.Run(mode, func(t *testing.T) {
			status := mode
			if mode == "complete" || mode == "claim_disagreement" || mode == "claim_interrupted" || mode == "claim_cancelled" {
				status = "supported"
			}
			var calls atomic.Int32
			var reviewCalls atomic.Int32
			inv := Investigation{ID: "inspection", Claim: "fixture text is present", ObservationIDs: []string{"observation-2"}, CounterObservationIDs: []string{"observation-2"}, Evidence: []string{"new"}, Counterevidence: []string{"new"}, PRContext: &PRInvestigationContext{ChangeSummary: "fixture change", Before: "fixture before", After: "fixture after"}}
			if status == "rejected" {
				inv.Claim = "fixture text is absent"
			}
			if mode == "complete" || mode == "claim_disagreement" || mode == "claim_interrupted" || mode == "claim_cancelled" {
				inv.Claim = "PR changes old to new"
				inv.ObservationIDs = []string{"observation-2", "observation-3"}
				inv.PRContext.Before = "old"
				inv.PRContext.After = "new"
				inv.PRContext.BeforeObservationIDs = []string{"observation-2"}
				inv.PRContext.AfterObservationIDs = []string{"observation-3"}
				inv.PRContext.Relationships = []InvestigationRelationship{{From: "a.go at BASE", To: "a.go at HEAD", Relation: "same changed path across fixed snapshots", Certainty: "cited", ObservationIDs: []string{"observation-2", "observation-3"}}}
			}
			for i, kind := range verificationKinds {
				inv.Plan = append(inv.Plan, InvestigationTask{ID: []string{"input", "change", "guard", "effect"}[i], Kind: kind, Question: "Inspect fixture source", Status: "checked", Reason: "fixture text read", ObservationIDs: []string{"observation-2"}})
			}
			s := testStore(t)
			var runner *Runner
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var request struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(request.Messages) > 0 && strings.HasPrefix(request.Messages[0].Content, claimVerificationPrompt[:40]) {
					msg := map[string]any{"role": "assistant"}
					finish := "stop"
					if reviewCalls.Add(1) == 1 {
						finish = "tool_calls"
						msg["tool_calls"] = []any{
							map[string]any{"id": "base", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.go","base":true,"start":1,"end":1}`}},
							map[string]any{"id": "head", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"a.go","base":false,"start":1,"end":1}`}},
						}
					} else {
						ids := []string{}
						for _, m := range request.Messages {
							if m.Role == "tool" {
								var out toolOutput
								json.Unmarshal([]byte(m.Content), &out)
								ids = append(ids, out.ObservationID)
							}
						}
						raw, _ := json.Marshal(claimVerificationInput{Verdict: "false", Reason: "Controlled disagreement fixture, not quality proof", Limitations: []string{}, ObservationIDs: ids})
						msg["content"] = string(raw)
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
					return
				}
				if mode == "claim_cancelled" && n == 5 {
					var id int64
					if err := s.DB.QueryRow(`SELECT id FROM platform_runs WHERE status='running'`).Scan(&id); err != nil {
						t.Error(err)
					} else if err := runner.Cancel(context.Background(), id, 0); err != nil {
						t.Error(err)
					}
					w.WriteHeader(400)
					return
				}
				if mode == "claim_interrupted" && n == 5 {
					w.WriteHeader(400)
					return
				}
				msg := map[string]any{"role": "assistant"}
				finish := "tool_calls"
				name, args := "read_file", `{"path":"a.go","start":1,"end":1}`
				recordRound := int32(2)
				if mode == "complete" || mode == "claim_disagreement" || mode == "claim_interrupted" || mode == "claim_cancelled" {
					recordRound = 3
					if n == 1 {
						args = `{"path":"a.go","base":true,"start":1,"end":1}`
					}
				}
				if n == recordRound || n == recordRound+1 {
					name = "record_hypothesis"
					copy := inv
					if n == recordRound+1 {
						name = "update_investigation"
						copy.Status = status
					}
					raw, _ := json.Marshal(copy)
					args = string(raw)
				}
				if n < recordRound+2 {
					msg["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}
				} else {
					finish = "stop"
					msg["content"] = `{"findings":[],"summary":"fixture summary","coverage_notes":[]}`
				}
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
			}))
			defer server.Close()
			ctx := context.Background()
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "one", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			auditor := &EinoAuditor{Repository: coverageComparisonRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 5, MaxToolCalls: 8, VerifyFindings: mode == "claim_disagreement" || mode == "claim_interrupted" || mode == "claim_cancelled"}}
			runner = &Runner{Store: s, Repository: coverageComparisonRepo{}, Auditor: auditor, Workers: 1, Timeout: 20 * time.Second}
			if err := runner.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer runner.Stop()
			id, _, err := runner.Submit(ctx, 1, 1, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantNotes, wantCalls := "incomplete", 3, int32(4)
			if mode == "complete" || mode == "claim_disagreement" || mode == "claim_interrupted" || mode == "claim_cancelled" {
				wantStatus, wantNotes, wantCalls = "succeeded", 0, 5
			}
			if mode == "claim_disagreement" {
				wantStatus, wantNotes, wantCalls = "incomplete", 1, 7
			}
			if mode == "claim_interrupted" {
				wantStatus, wantNotes, wantCalls = "failed", 2, 5
			}
			if mode == "claim_cancelled" {
				wantStatus, wantNotes, wantCalls = "cancelled", 2, 5
			}
			waitStatus(t, s, id, wantStatus, runner.Timeout+time.Second)
			run, err := s.Run(ctx, id)
			if err != nil || calls.Load() != wantCalls || len(run.Result.Findings) != 0 || len(run.Result.Investigations) != 1 || len(run.Result.CoverageNotes) != wantNotes {
				t.Fatal(err, calls.Load(), run.Result)
			}
			if run.Result.Investigations[0].Status != status || (mode != "complete" && mode != "claim_disagreement" && mode != "claim_interrupted" && mode != "claim_cancelled" && run.Result.Investigations[0].PRContext.BeforeObservationIDs != nil) {
				t.Fatal("coverage fabricated resolution or source links", run.Result)
			}
			export := BuildSARIF(run, nil)["runs"].([]sarifObject)[0]
			invocation := export["invocations"].([]sarifObject)[0]
			if invocation["executionSuccessful"] != (mode == "complete") || len(invocation["toolExecutionNotifications"].([]sarifObject)) != wantNotes || export["properties"].(sarifObject)["runtimeReproduced"] != false {
				t.Fatal("persisted coverage lost in SARIF", export)
			}
			if mode == "claim_disagreement" {
				v := run.Result.Investigations[0].ClaimVerification
				if v == nil || v.Status != "disagreed" || v.AssessedClaim != inv.Claim || len(v.ObservationIDs) != 2 || run.Result.Investigations[0].Status != "supported" || reviewCalls.Load() != 2 {
					t.Fatal("review/primary persistence lost", run)
				}
				reviews := export["properties"].(sarifObject)["investigationClaimReviews"].([]sarifObject)
				if len(reviews) != 1 || reviews[0]["verification"].(*ClaimVerification).Status != "disagreed" {
					t.Fatal("SARIF review lost", reviews)
				}
			}
			if mode == "claim_interrupted" || mode == "claim_cancelled" {
				v := run.Result.Investigations[0].ClaimVerification
				if v == nil || v.Status != "unavailable" || v.Verdict != "" || run.Result.Investigations[0].Status != "supported" || reviewCalls.Load() != 0 {
					t.Fatal("skipped review disappeared or fabricated verdict", run)
				}
			}
			notes := strings.Join(run.Result.CoverageNotes, " ")
			if mode != "complete" && mode != "claim_disagreement" && mode != "claim_interrupted" && mode != "claim_cancelled" && (!strings.Contains(notes, "BASE") || !strings.Contains(notes, "HEAD") || !strings.Contains(notes, "relationships")) {
				t.Fatal("recording gaps lost in final persistence", notes)
			}
		})
	}
}
