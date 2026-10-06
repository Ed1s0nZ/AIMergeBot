package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	em "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type recordingFixtureModel struct {
	calls    *atomic.Int32
	generate func(context.Context, []*schema.Message, int32) (*schema.Message, error)
}

func (m *recordingFixtureModel) Generate(ctx context.Context, in []*schema.Message, _ ...em.Option) (*schema.Message, error) {
	return m.generate(ctx, in, m.calls.Add(1))
}
func (m *recordingFixtureModel) Stream(context.Context, []*schema.Message, ...em.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("unused")
}
func (m *recordingFixtureModel) WithTools([]*schema.ToolInfo) (em.ToolCallingChatModel, error) {
	return &recordingFixtureModel{calls: m.calls, generate: m.generate}, nil
}

func TestPrimaryFinalizationSharedCapAndLastToolCannotExecute(t *testing.T) {
	tools := &auditTools{}
	calls := &atomic.Int32{}
	inner := &recordingFixtureModel{calls: calls, generate: func(_ context.Context, in []*schema.Message, n int32) (*schema.Message, error) {
		if !strings.Contains(in[0].Content, fmt.Sprintf("decision %d of 3", n)) {
			t.Error("incorrect cumulative budget", in[0].Content)
		}
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call", Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`}}}}, nil
	}}
	root := newPrimaryRecordingModel(inner, tools, 3, "trusted")
	bound, err := root.WithTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	rebound, err := bound.WithTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	input := []*schema.Message{schema.SystemMessage("old"), schema.UserMessage("source")}
	for i, m := range []em.ToolCallingChatModel{root, bound, rebound, root} {
		msg, err := m.Generate(context.Background(), input)
		if i < 2 {
			if err != nil || len(msg.ToolCalls) != 1 {
				t.Fatal(msg, err)
			}
		} else if !errors.Is(err, compose.ErrExceedMaxSteps) || msg != nil {
			t.Fatal("budget/last tools escaped", msg, err)
		}
	}
	if calls.Load() != 3 || input[0].Content != "old" {
		t.Fatal(calls.Load(), input)
	}
	if _, err := rebound.Stream(context.Background(), input); err == nil || calls.Load() != 3 {
		t.Fatal("stream budget bypass")
	}
}

func TestPrimaryFinalizationOneAttemptAndCandidateRetentionOnFailure(t *testing.T) {
	for _, mode := range []string{"repeat_final", "transport", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			tools, _, f := contextRecordingFixture(t)
			f.InvestigationID = "inspection"
			f.ObservationIDs = tools.ledger["inspection"].ObservationIDs
			raw := `{"findings":[],"summary":"draft","coverage_notes":[]}`
			// Include one valid candidate; the model-provided context cannot grant facts.
			draft := AuditResult{Findings: []Finding{f}, Summary: "draft", CoverageNotes: []string{}}
			raw = mustRecordingJSON(t, draft)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := &atomic.Int32{}
			inner := &recordingFixtureModel{calls: calls, generate: func(_ context.Context, in []*schema.Message, n int32) (*schema.Message, error) {
				if n == 2 {
					if !strings.Contains(in[len(in)-1].Content, "remaining ORIGINAL") || !strings.Contains(in[0].Content, "decision 2 of 5") {
						t.Error("missing feedback or actual cap", in)
					}
					if tools.acceptedFindings() == nil || len(tools.acceptedFindings()) != 1 {
						t.Error("draft candidate not saved before next request")
					}
					if mode == "transport" {
						return nil, errors.New("synthetic transport failure")
					}
					if mode == "cancel" {
						cancel()
						return nil, context.Canceled
					}
				}
				return schema.AssistantMessage(raw, nil), nil
			}}
			model := newPrimaryRecordingModel(inner, tools, 5, "trusted")
			_, err := model.Generate(ctx, []*schema.Message{schema.SystemMessage("trusted")})
			if (err != nil) != (mode != "repeat_final") || len(tools.acceptedFindings()) != 1 {
				t.Fatal(err, tools.acceptedFindings())
			}
			if mode == "repeat_final" {
				_, err = model.Generate(ctx, []*schema.Message{schema.SystemMessage("trusted")})
				if err != nil || calls.Load() != 3 {
					t.Fatal("second recording attempt repeated", err, calls.Load())
				}
			} else if calls.Load() != 2 {
				t.Fatal(calls.Load())
			}
		})
	}
}

func TestPrimaryFinalizationSkipsWithoutActualOpportunity(t *testing.T) {
	for _, mode := range []string{"no_source", "no_pr_scope", "complete", "no_tools", "no_decisions", "invalid_json", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			tools, a, _ := contextRecordingFixture(t)
			limit := 5
			if mode == "no_pr_scope" {
				tools.scope = DiffScope{}
			}
			if mode == "no_source" {
				tools.trace = nil
			}
			if mode == "complete" {
				out, _ := tools.recordPRContext(context.Background(), a)
				if out.Error != "" {
					t.Fatal(out)
				}
				v := tools.ledger[a.ID]
				v.PRContext.Relationships[0].Certainty = "cited"
				tools.ledger[a.ID] = v
			}
			if mode == "no_tools" {
				tools.maxCalls = tools.calls
			}
			if mode == "no_decisions" {
				limit = 2
			}
			calls := &atomic.Int32{}
			inner := &recordingFixtureModel{calls: calls, generate: func(context.Context, []*schema.Message, int32) (*schema.Message, error) {
				s := `{"findings":[],"summary":"final","coverage_notes":[]}`
				if mode == "invalid_json" {
					s = "not json"
				}
				return schema.AssistantMessage(s, nil), nil
			}}
			model := newPrimaryRecordingModel(inner, tools, limit, "trusted")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			_, err := model.Generate(ctx, []*schema.Message{schema.SystemMessage("trusted")})
			want := int32(1)
			if mode == "cancelled" {
				want = 0
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != want {
				t.Fatal(mode, calls.Load(), want)
			}
		})
	}
}

func TestPrimaryFinalizationRejectsUnsupportedDraftAndPreservesLedger(t *testing.T) {
	tools, _, f := contextRecordingFixture(t)
	f.Evidence = "invented anchor"
	f.InvestigationID = "inspection"
	before := tools.investigations()
	calls := &atomic.Int32{}
	inner := &recordingFixtureModel{calls: calls, generate: func(context.Context, []*schema.Message, int32) (*schema.Message, error) {
		return schema.AssistantMessage(mustRecordingJSON(t, AuditResult{Findings: []Finding{f}, Summary: "draft", CoverageNotes: []string{}}), nil), nil
	}}
	m := newPrimaryRecordingModel(inner, tools, 4, "trusted")
	_, err := m.Generate(context.Background(), []*schema.Message{schema.SystemMessage("trusted")})
	if err != nil || len(tools.acceptedFindings()) != 0 || !reflect.DeepEqual(before, tools.investigations()) || calls.Load() != 2 {
		t.Fatal(err, tools.acceptedFindings(), calls.Load())
	}
}

func mustRecordingJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPrimaryFinalizationRewritesFollowUpAndRetainsControlAfterCompression(t *testing.T) {
	for _, mode := range []string{"compressed", "compression_stop"} {
		t.Run(mode, func(t *testing.T) {
			tools, _, _ := contextRecordingFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := &atomic.Int32{}
			rewrites := 0
			inner := &recordingFixtureModel{calls: calls, generate: func(_ context.Context, in []*schema.Message, n int32) (*schema.Message, error) {
				if n == 2 && (!strings.Contains(in[0].Content, "remaining ORIGINAL") || len(in) != 2 || in[1].Content != "untrusted compressed navigation") {
					t.Error("follow-up bypassed compression or lost server request", in)
				}
				return schema.AssistantMessage(`{"findings":[],"summary":"draft","coverage_notes":[]}`, nil), nil
			}}
			rewrite := func(_ context.Context, in []*schema.Message) []*schema.Message {
				rewrites++
				if mode == "compression_stop" {
					cancel()
					return in
				}
				return []*schema.Message{schema.SystemMessage("old prompt"), schema.UserMessage("untrusted compressed navigation")}
			}
			model := newPrimaryRecordingModel(inner, tools, 5, "trusted", rewrite)
			bound, err := model.WithTools(nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = bound.Generate(ctx, []*schema.Message{schema.SystemMessage("trusted")})
			want := int32(2)
			if mode == "compression_stop" {
				want = 1
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if rewrites != 1 || calls.Load() != want {
				t.Fatal(rewrites, calls.Load(), want)
			}
		})
	}
}
