package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCompressionFinalizePreservesAuthorityAndPairs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools := &auditTools{snap: Snapshot{BaseSHA: "fixed-base", HeadSHA: "fixed-head"}, ledger: map[string]Investigation{"h": {ID: "h", Status: "investigating", Claim: "guard unknown", NextSteps: []string{"read callers"}}}, findings: map[string]Finding{"f": {ID: "f", Title: "retained"}}, trace: []ToolTrace{{Name: "read_file", ObservationID: "observation-1", Arguments: `{"path":"guard.any"}`}}}
	initial := []*schema.Message{{Role: schema.System, Content: "immutable policy"}, {Role: schema.User, Content: "immutable pinned diff"}}
	c := &auditCompression{tools: tools, initial: initial, cancel: cancel}
	call := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "read"}}}
	response := &schema.Message{Role: schema.Tool, ToolCallID: "read", Content: "source"}
	out, err := c.finalize(ctx, append(append([]*schema.Message{}, initial...), call, response), &schema.Message{Role: schema.Assistant, Content: "hypothesis navigation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 5 || out[0] != initial[0] || out[1] != initial[1] || out[3] != call || out[4] != response {
		t.Fatal("immutable input or tool pair lost")
	}
	for _, want := range []string{"evidence_eligible=false", "fixed-head", "read callers", "retained", "observation-1"} {
		if !strings.Contains(out[2].Content, want) {
			t.Errorf("state missing %s", want)
		}
	}
	if len(tools.trace) != 1 || len(tools.ledger) != 1 || len(tools.findings) != 1 {
		t.Fatal("source ledger modified")
	}
	for _, summary := range []*schema.Message{nil, {Content: " "}, {Content: strings.Repeat("x", 16*1024+1)}, {Content: "bad", ToolCalls: []schema.ToolCall{{ID: "fake"}}}} {
		if _, err := c.finalize(ctx, initial, summary); !errors.Is(err, ErrContextCompression) {
			t.Fatal("invalid summary accepted", err)
		}
	}
	response.Content = strings.Repeat("x", 40*1024)
	out, err = c.finalize(ctx, append(append([]*schema.Message{}, initial...), call, response), &schema.Message{Content: "navigation"})
	if err != nil || len(out) != 3 {
		t.Fatal("large exchange was split or preserved", err)
	}
	c.initial = []*schema.Message{{Role: schema.System, Content: "policy"}, {Role: schema.User, Content: strings.Repeat("x", 80*1024)}}
	response.Content = strings.Repeat("x", 30*1024)
	out, err = c.finalize(ctx, append(append([]*schema.Message{}, c.initial...), call, response), &schema.Message{Content: strings.Repeat("s", 8*1024)})
	if err != nil || len(out) != 3 {
		t.Fatal("optional tail exceeded total capacity instead of being summarized", err)
	}
	c.initial = initial
	tools.ledger["huge"] = Investigation{Claim: strings.Repeat("x", 49*1024)}
	if _, err = c.finalize(ctx, initial, &schema.Message{Content: "navigation"}); !errors.Is(err, ErrContextCompression) {
		t.Fatal("ledger silently omitted", err)
	}
}

func TestEinoLongInvestigationCompression(t *testing.T) {
	for _, mode := range []string{"success", "failure", "empty", "unknown_usage", "threshold"} {
		t.Run(mode, func(t *testing.T) {
			primary, summaries := 0, 0
			compressed := false
			var pinned string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages       []*schema.Message `json:"messages"`
					ResponseFormat json.RawMessage   `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				summary := len(req.ResponseFormat) == 0 || string(req.ResponseFormat) == "null"
				message := map[string]any{"role": "assistant"}
				finish := "stop"
				usage := map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}
				if summary {
					summaries++
					if mode == "failure" {
						w.WriteHeader(400)
						return
					}
					message["content"] = "Continue pinned source investigation, observations retained; read remaining caller."
					if mode == "empty" {
						message["content"] = ""
					}
					if mode == "unknown_usage" {
						usage = nil
					}
					if mode == "threshold" {
						usage = map[string]int{"prompt_tokens": 1000, "completion_tokens": 1, "total_tokens": 1001}
					}
				} else {
					primary++
					if len(req.Messages) < 2 || !strings.HasPrefix(req.Messages[0].Content, "You are a security code reviewer") {
						t.Error("system policy lost")
					}
					if pinned == "" {
						pinned = req.Messages[1].Content
					} else if pinned != req.Messages[1].Content {
						t.Error("pinned initial task changed")
					}
					for _, m := range req.Messages {
						if strings.Contains(m.Content, "Compressed investigation navigation") {
							compressed = true
						}
					}
					if primary == 12 {
						message["content"] = `{"findings":[],"summary":"static integration fixture","coverage_notes":[]}`
					} else {
						finish = "tool_calls"
						message["tool_calls"] = []any{map[string]any{"id": "read", "type": "function", "function": map[string]string{"name": "read_file", "arguments": `{"path":"file.any","start":1,"end":200}`}}}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}, "usage": usage})
			}))
			defer server.Close()
			a := &EinoAuditor{Repository: fixtureRepo{files: map[string]string{"file.any": strings.Repeat(strings.Repeat("a", 60)+"\n", 200)}}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "synthetic", MaxSteps: 16, MaxTokens: 1000}}
			result, trace, err := a.Audit(context.Background(), Snapshot{BaseSHA: "fixed-base", HeadSHA: "fixed-head"}, DiffScope{})
			if summaries == 0 {
				t.Fatal("compression never triggered")
			}
			if mode == "success" {
				if err != nil || primary != 12 || !compressed {
					t.Fatal("did not continue after compression", primary, err)
				}
			} else {
				if err == nil || primary >= 12 || len(result.CoverageNotes) == 0 {
					t.Fatal("compression failure continued or clean", primary, err)
				}
				if (mode == "failure" || mode == "empty") && !errors.Is(err, ErrContextCompression) {
					t.Fatal("missing compression failure", err)
				}
			}
			usageSummary := SummarizeModelUsage(trace, ModelBudgetSettings{}, true)
			if usageSummary.Calls != primary+summaries {
				t.Fatal("usage duplicated or missing", usageSummary.Calls, primary, summaries)
			}
			stageCalls := 0
			for _, stage := range usageSummary.Stages {
				if stage.Stage == "compression" {
					stageCalls = stage.Calls
				}
			}
			if stageCalls != summaries {
				t.Fatal("compression stage missing", stageCalls, summaries)
			}
		})
	}
}

func TestInterruptedAuditRetainsInvestigationLedger(t *testing.T) {
	for _, final := range []string{"", "invalid-json"} {
		t.Run(final, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				message := map[string]any{"role": "assistant", "content": final}
				finish := "stop"
				if calls == 1 {
					finish = "tool_calls"
					delete(message, "content")
					message["tool_calls"] = []any{map[string]any{"id": "record", "type": "function", "function": map[string]string{"name": "record_hypothesis", "arguments": `{"id":"retained-hypothesis","claim":"caller permissions unresolved","next_steps":["read caller"]}`}}}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": message}}})
			}))
			defer server.Close()
			a := &EinoAuditor{Repository: fixtureRepo{}, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "synthetic", MaxSteps: 4}}
			result, _, err := a.Audit(context.Background(), Snapshot{}, DiffScope{})
			if err == nil || len(result.Investigations) != 1 || result.Investigations[0].ID != "retained-hypothesis" || len(result.Investigations[0].NextSteps) != 1 {
				t.Fatal("interrupted audit lost investigation", err, result.Investigations)
			}
		})
	}
}
