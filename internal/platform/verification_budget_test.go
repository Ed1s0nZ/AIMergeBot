package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	eo "github.com/cloudwego/eino-ext/components/model/openai"
)

func TestVerificationBudgetRemainsOneBoundedPool(t *testing.T) {
	b := newVerificationBudget(context.Background())
	defer b.cancel()
	if b.remaining != 40 || b.nextLimit() != 10 {
		t.Fatal(b.remaining)
	}
	deadline, ok := b.ctx.Deadline()
	if !ok || time.Until(deadline) > 60*time.Second || time.Until(deadline) < 59*time.Second {
		t.Fatal("phase deadline changed", deadline)
	}
	b.consume(3, 10)
	if b.remaining != 37 {
		t.Fatal(b.remaining)
	}
	for n := 0; n < 3; n++ {
		b.consume(11, 10)
	}
	if b.nextLimit() != 7 {
		t.Fatal("tail limit", b.remaining)
	}
	b.consume(9, 7)
	if b.remaining != 0 || b.nextLimit() != 0 {
		t.Fatal("pool replenished", b.remaining)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	short := newVerificationBudget(ctx)
	defer short.cancel()
	if short.nextLimit() != 0 {
		t.Fatal("last ten seconds consumed")
	}
	b2 := newVerificationBudget(context.Background())
	b2.cancel()
	if b2.nextLimit() != 0 {
		t.Fatal("canceled pool allocated calls")
	}
}

func TestVerificationSharedPoolAcrossSDKConsumersDoesNotReset(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []struct{ Function struct{ Name string } }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		for _, tool := range req.Tools {
			if tool.Function.Name == "update_investigation" || tool.Function.Name == "record_hypothesis" {
				t.Error("mutation tool exposed")
			}
		}
		n := requests.Add(1)
		msg := map[string]any{"role": "assistant"}
		finish := "stop"
		if n == 1 {
			calls := []any{}
			for k := 0; k < 2; k++ {
				calls = append(calls, map[string]any{"id": fmt.Sprint(k), "type": "function", "function": map[string]string{"name": "list_files", "arguments": `{"page":1}`}})
			}
			msg["tool_calls"] = calls
			finish = "tool_calls"
		} else {
			msg["content"] = `{"status":"inconclusive","reason":"No fresh source inspected in this budget fixture","claim_coverage":"unknown","limitations":[],"observation_ids":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
	}))
	defer server.Close()
	model, err := eo.NewChatModel(context.Background(), &eo.ChatModelConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	e := EinoAuditor{Config: AgentConfig{Model: "fixture"}}
	parent := &auditTools{repo: repo, snap: snap}
	b := newVerificationBudget(context.Background())
	defer b.cancel()
	b.remaining = 2
	first := AuditResult{Summary: "primary retained", Findings: []Finding{f}}
	e.verifyFindingsWithBudget(context.Background(), &first, parent, model, b)
	if requests.Load() != 2 || b.remaining != 0 || b.ctx.Err() != nil || first.Findings[0].Verification == nil || first.Findings[0].Verification.Status != "inconclusive" {
		t.Fatal("first consumer budget/ownership", requests.Load(), b.remaining, first)
	}
	second := AuditResult{Summary: "second primary retained", Findings: []Finding{f}}
	e.verifyFindingsWithBudget(context.Background(), &second, parent, model, b)
	if requests.Load() != 2 || second.Findings[0].Verification.Status != "unavailable" || len(second.CoverageNotes) == 0 || second.Summary != "second primary retained" {
		t.Fatal("second consumer allocated a fresh pool or lost discovery", requests.Load(), second)
	}
}
