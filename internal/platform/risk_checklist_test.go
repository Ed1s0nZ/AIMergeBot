package platform

import (
	"context"
	"strings"
	"testing"
)

func TestRiskChecklistIsGuidanceNotEvidence(t *testing.T) {
	for _, category := range []string{"access_control", "tenant_isolation", "business_state", "concurrency", "injection"} {
		t.Run(category, func(t *testing.T) {
			tools := &auditTools{}
			out, err := tools.riskChecklist(context.Background(), riskChecklistArgs{Category: category})
			if err != nil || out.Error != "" || out.EvidenceEligible || isSourceTool("get_risk_checklist") || !strings.Contains(out.Text, "Counterexample:") {
				t.Fatalf("unexpected guidance: %+v %v", out, err)
			}
			if tools.calls != 1 || len(tools.trace) != 1 {
				t.Fatal("guidance did not use normal budget/trace")
			}
			tools.mu.Lock()
			err = tools.validateObservationIDs([]string{out.ObservationID})
			tools.mu.Unlock()
			if err == nil {
				t.Fatal("knowledge accepted as source observation")
			}
		})
	}
}

func TestRiskChecklistInvalidCancelledAndBudget(t *testing.T) {
	tools := &auditTools{maxCalls: 1}
	out, _ := tools.riskChecklist(context.Background(), riskChecklistArgs{Category: "../../repository"})
	if out.Error == "" || out.Text != "" {
		t.Fatalf("unknown category accepted: %+v", out)
	}
	out, _ = tools.riskChecklist(context.Background(), riskChecklistArgs{Category: "injection"})
	if out.Error == "" || out.Text != "" {
		t.Fatal("knowledge bypassed call budget")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, _ = (&auditTools{}).riskChecklist(ctx, riskChecklistArgs{Category: "injection"})
	if out.Error == "" || out.Text != "" {
		t.Fatal("cancelled checklist succeeded")
	}
}

func TestRiskChecklistRegisteredInFreshReadTools(t *testing.T) {
	registered, err := (&auditTools{}).register()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range readOnlyTools(context.Background(), registered) {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "get_risk_checklist" {
			found = true
		}
	}
	if !found {
		t.Fatal("checklist unavailable to independent review")
	}
}
