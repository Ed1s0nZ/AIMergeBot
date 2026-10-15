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

	"github.com/cloudwego/eino/schema"
)

// Exercise the real stage builders and SDK transport, not a model stub whose
// failure could hide a lost context. A positive control must reach HTTP first.
func TestRepositoryFenceReachesEveryModelStageHTTP(t *testing.T) {
	for _, stage := range []string{"primary", "verification", "claim", "compression", "group_synthesis"} {
		t.Run(stage, func(t *testing.T) {
			for _, blocked := range []bool{false, true} {
				var requests, checks atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					content := `{"findings":[],"summary":"static result","coverage_notes":[]}`
					if stage == "compression" {
						content = "bounded navigation summary"
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 3, "total_tokens": 6}, "choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
				}))
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				svc, _ := activationFixture(t, credentialActivationOptions{})
				descriptor, err := svc.captureLegacyCredential(ctx)
				if err != nil {
					cancel()
					server.Close()
					t.Fatal(err)
				}
				if blocked {
					cfg := svc.Snapshot()
					cfg.GitLab.Token = "PRIVATE-ROTATED"
					if err = svc.Save(cfg); err != nil {
						cancel()
						server.Close()
						t.Fatal(err)
					}
				}
				ctx = withRepositoryFence(ctx, func(check context.Context) error {
					checks.Add(1)
					return descriptor.validate(check)
				})
				ctx = withModelBudget(ctx, 0)
				repo, snap, finding, _ := sequenceFixture()
				cfg := AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "primary", VerificationModel: "secondary", MaxSteps: 2, VerifyFindings: true}
				e := &EinoAuditor{Repository: repo, Config: cfg}
				parent := &auditTools{repo: repo, snap: snap, scope: DiffScope{Included: []string{finding.File}, Added: map[string]map[int]bool{finding.File: {2: true}}}, cache: map[string]string{}}
				switch stage {
				case "primary":
					e.Audit(ctx, snap, parent.scope)
				case "verification":
					result := AuditResult{Findings: []Finding{finding}}
					e.verifyFindings(ctx, &result, parent, nil)
				case "claim":
					result := AuditResult{Investigations: []Investigation{{ID: "claim-1", Status: "rejected", Claim: "fixed commit claim"}}}
					pool := newVerificationBudget(ctx)
					e.verifyInvestigationClaims(ctx, &result, parent, nil, pool)
					pool.cancel()
				case "compression":
					initial := []*schema.Message{{Role: schema.System, Content: "untrusted static investigation"}}
					c, err := newAuditCompression(ctx, cfg, parent, nil, initial, func() {})
					if err != nil {
						cancel()
						server.Close()
						t.Fatal(err)
					}
					messages := append([]*schema.Message{}, initial...)
					for i := 0; i < 20; i++ {
						messages = append(messages, &schema.Message{Role: schema.User, Content: strings.Repeat("navigation ", 2000)}, &schema.Message{Role: schema.Assistant, Content: "continue source investigation"})
					}
					c.rewrite(ctx, messages)
				case "group_synthesis":
					// No fragments: isolate the aggregate stage's real model builder.
					// This is not an end-to-end grouped audit acceptance test.
					e.AuditGroups(ctx, snap, AuditPlan{})
				}
				cancel()
				server.Close()
				if checks.Load() == 0 || (blocked && requests.Load() != 0) || (!blocked && requests.Load() == 0) {
					t.Fatal("stage did not enforce actual HTTP fence", "blocked", blocked, "checks", checks.Load(), "requests", requests.Load())
				}
			}
		})
	}
}
