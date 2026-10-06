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

type inventoryFixtureRepo struct {
	fixtureRepo
	listError error
	paged     bool
}

func (r inventoryFixtureRepo) ListFiles(ctx context.Context, snap Snapshot, page int) ([]string, bool, error) {
	if r.listError != nil {
		return nil, false, r.listError
	}
	if r.paged {
		if page == 1 {
			return []string{"main.any"}, true, nil
		}
		return []string{"caller.unknown", "workflow.contract"}, false, nil
	}
	return r.fixtureRepo.ListFiles(ctx, snap, page)
}

func TestPrimaryInventoryPreflightIsBudgetedNonSourceAndRecoverable(t *testing.T) {
	ctx := context.Background()
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	tools := &auditTools{snap: snap, repo: inventoryFixtureRepo{paged: true}, maxCalls: 2}
	nav, err := tools.primaryRepositoryNavigation(ctx)
	if err != nil || !strings.Contains(nav, `"more":true`) || len(tools.trace) != 1 || tools.calls != 1 || tools.trace[0].Name != "list_files" || len(tools.unresolved()) != 1 {
		t.Fatal("initial partial inventory not budgeted/persisted", err, nav)
	}
	if tools.validateObservationIDs([]string{tools.trace[0].ObservationID}) == nil {
		t.Fatal("inventory cited as source")
	}
	tools.mu.Lock()
	state, count := tools.primaryHeadInventoryLocked()
	tools.mu.Unlock()
	if state != "partial" || count != 1 {
		t.Fatal(state, count)
	}
	tools.list(ctx, listArgs{Page: 2})
	tools.mu.Lock()
	state, count = tools.primaryHeadInventoryLocked()
	tools.mu.Unlock()
	if state != "complete" || count != 3 || len(tools.unresolved()) != 0 || tools.calls != 2 {
		t.Fatal("page recovery incorrect", state, count)
	}
	out, _ := tools.list(ctx, listArgs{Page: 3})
	if out.Error == "" || !strings.Contains(out.Error, "budget") {
		t.Fatal("preflight bypassed tool budget", out)
	}

	tools = &auditTools{snap: snap, repo: inventoryFixtureRepo{listError: errors.New("PRIVATE PROVIDER DETAIL")}}
	nav, err = tools.primaryRepositoryNavigation(ctx)
	if err != nil || strings.Contains(nav, "PRIVATE") || len(tools.unresolved()) != 1 {
		t.Fatal("ordinary listing failure aborted or exposed error", nav, err)
	}
	tools.repo = inventoryFixtureRepo{fixtureRepo: fixtureRepo{files: map[string]string{"caller.any": "fixture"}}}
	tools.list(ctx, listArgs{Page: 1})
	if len(tools.unresolved()) != 0 || tools.trace[0].Error == "" {
		t.Fatal("repair lost history or pending remained")
	}
}

func TestPrimaryInventoryProjectionRejectsForeignStaleAndSkippedPages(t *testing.T) {
	for _, mode := range []string{"foreign", "stale", "stage", "source_flag", "wrong_id", "missing_first", "large_page"} {
		t.Run(mode, func(t *testing.T) {
			tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}, repo: inventoryFixtureRepo{fixtureRepo: fixtureRepo{files: map[string]string{"PRIVATE PATH": "fixture"}}}}
			tools.list(context.Background(), listArgs{Page: 1})
			tr := &tools.trace[0]
			var out toolOutput
			json.Unmarshal([]byte(tr.Output), &out)
			switch mode {
			case "foreign":
				out.RepositoryID = 2
			case "stale":
				out.HeadSHA = "stale"
			case "stage":
				tr.Stage = "verification"
			case "source_flag":
				out.EvidenceEligible = true
			case "wrong_id":
				out.ObservationID = "wrong"
			case "missing_first":
				tr.Arguments = `{"page":2}`
			case "large_page":
				tr.Arguments = `{"page":2147483647}`
			}
			raw, _ := json.Marshal(out)
			tr.Output = string(raw)
			nav := tools.primaryProgressNavigation()
			if strings.Contains(nav, `"primary_head_inventory_state":"complete"`) || strings.Contains(nav, "PRIVATE") {
				t.Fatal("invalid inventory or path in system navigation", nav)
			}
		})
	}
}

func TestPrimaryInventorySDKContainsUnchangedPathsAndStopsBeforeModel(t *testing.T) {
	for _, mode := range []string{"success", "ordinary_error", "repository_unavailable", "checkpoint_error", "checkpoint_conflict", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			var sawInventory atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					Messages []struct{ Role, Content string } `json:"messages"`
				}
				json.NewDecoder(r.Body).Decode(&request)
				for _, msg := range request.Messages {
					if msg.Role == "system" && (strings.Contains(msg.Content, "caller.unknown") || strings.Contains(msg.Content, "workflow.contract")) {
						t.Error("path entered trusted system message")
					}
					if strings.Contains(msg.Content, "PRIVATE PROVIDER DETAIL") {
						t.Error("raw preflight error exposed")
					}
					if msg.Role == "user" && strings.Contains(msg.Content, "caller.unknown") && strings.Contains(msg.Content, "workflow.contract") && strings.Contains(msg.Content, `"evidence_eligible":false`) {
						sawInventory.Store(true)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": `{"findings":[],"summary":"controlled initial inventory receipt","coverage_notes":[]}`}}}})
			}))
			defer server.Close()
			repo := inventoryFixtureRepo{fixtureRepo: fixtureRepo{files: map[string]string{"main.any": "fixture", "caller.unknown": "fixture", "workflow.contract": "fixture"}}}
			cfg := AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", Manifest: "main.any", MaxSteps: 3, MaxToolCalls: 4}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "ordinary_error":
				repo.listError = errors.New("PRIVATE PROVIDER DETAIL")
			case "repository_unavailable":
				repo.listError = ErrRepositoryUnavailable
			case "checkpoint_error":
				cfg.Progress = func(AuditResult, []ToolTrace) error { return errors.New("PRIVATE CHECKPOINT DETAIL") }
			case "checkpoint_conflict":
				cfg.Progress = func(AuditResult, []ToolTrace) error { return ErrConflict }
			case "canceled":
				cancel()
			}
			a := EinoAuditor{Repository: repo, Config: cfg}
			_, trace, err := a.Audit(ctx, Snapshot{BaseSHA: "base", HeadSHA: "head"}, DiffScope{})
			if mode == "success" || mode == "ordinary_error" {
				if err != nil || calls.Load() != 1 || mode == "success" && !sawInventory.Load() || len(trace) < 1 || trace[0].Name != "list_files" {
					t.Fatal("initial navigation not consumed", calls.Load(), err)
				}
			} else if err == nil || calls.Load() != 0 {
				t.Fatal("model called after fatal preflight", calls.Load(), err)
			}
		})
	}
}
