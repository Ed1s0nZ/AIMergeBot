package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Derived from the independent V39-001 overlay counterexample.
func TestGroupedCheckpointFailureStopsBeforeLaterModels(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "ordinary"
		failure := errors.New("transient checkpoint unavailable")
		if conflict {
			name = "conflict"
			failure = ErrConflict
		}
		t.Run(name, func(t *testing.T) {
			for _, failAt := range []int32{1, 2} {
				var models, checkpoints atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { models.Add(1); w.WriteHeader(400) }))
				repo, snap, _, _ := sequenceFixture()
				auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: "synthetic", BaseURL: server.URL, Model: "fixture", MaxSteps: 2, MaxToolCalls: 8, Progress: func(AuditResult, []ToolTrace) error {
					if checkpoints.Add(1) == failAt {
						return failure
					}
					return nil
				}}}
				result, trace, err := auditor.AuditGroups(context.Background(), snap, AuditPlan{Groups: []AuditGroup{{ID: "group-1"}, {ID: "group-2"}}})
				server.Close()
				// Both startup checkpoints precede an actual SDK HTTP request.
				// A subsequent successful callback must never erase either failure.
				wantModels := int32(0)
				if !errors.Is(err, failure) || models.Load() != wantModels || checkpoints.Load() != failAt {
					t.Fatalf("failure %d did not stop: models=%d checkpoints=%d err=%v", failAt, models.Load(), checkpoints.Load(), err)
				}
				if len(trace) == 0 || trace[0].Name != "list_files" || result.AuditGroups[1].Status != "unprocessed" {
					t.Fatal("trace lost or later group processed", trace, result.AuditGroups)
				}
			}
		})
	}
}
