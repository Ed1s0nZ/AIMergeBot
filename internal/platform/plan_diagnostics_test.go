package platform

import (
	"strings"
	"testing"
)

func TestPlanSourceDiagnosticsAreActionableAndBounded(t *testing.T) {
	secret := "untrusted-private-value"
	a := Investigation{Plan: []InvestigationTask{{ID: "task", Kind: "guards", Question: "inspect guard", Status: "checked", Reason: "fixture", ObservationIDs: []string{secret}}}}
	err := prepareInvestigationPlan(&a, Investigation{})
	if err == nil || !strings.Contains(err.Error(), "top-level observation_ids or counter_observation_ids") || strings.Contains(err.Error(), secret) {
		t.Fatal("unsafe or unactionable missing-link message", err)
	}
	a.ObservationIDs = []string{secret}
	a.Plan[0].ObservationIDs = []string{secret, secret}
	err = prepareInvestigationPlan(&a, Investigation{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), secret) {
		t.Fatal("duplicate conflated or reflected", err)
	}
	a.Plan[0].ObservationIDs = []string{secret}
	if err = prepareInvestigationPlan(&a, Investigation{}); err != nil {
		t.Fatal("valid linkage rejected", err)
	}
}
