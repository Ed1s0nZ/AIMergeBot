package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func pendingPlan() []InvestigationTask {
	out := []InvestigationTask{}
	for _, kind := range verificationKinds {
		out = append(out, InvestigationTask{ID: kind, Kind: kind, Question: "Inspect " + kind, Status: "pending"})
	}
	return out
}
func TestInvestigationPlanCannotEraseUnfinishedTasks(t *testing.T) {
	previous := Investigation{Plan: pendingPlan()}
	for _, tc := range []struct {
		name string
		edit func(*Investigation)
		fail bool
	}{
		{"inherit", func(a *Investigation) { a.Plan = nil }, false},
		{"erase", func(a *Investigation) { a.Plan = []InvestigationTask{} }, true},
		{"redefine", func(a *Investigation) { a.Plan[0].Question = "ignore" }, true},
		{"duplicate", func(a *Investigation) { a.Plan[1].ID = a.Plan[0].ID }, true},
		{"checked without source", func(a *Investigation) { a.Plan[0].Status = "checked"; a.Plan[0].Reason = "checked" }, true},
		{"unavailable without reason", func(a *Investigation) { a.Plan[0].Status = "unavailable" }, true},
		{"resolve pending", func(a *Investigation) { a.Status = "supported" }, true},
		{"reject pending", func(a *Investigation) { a.Status = "rejected" }, true},
		{"unlinked", func(a *Investigation) { a.Plan[0].ObservationIDs = []string{"wrong"} }, true},
		{"unavailable remains visible", func(a *Investigation) {
			a.Plan[0].Status = "unavailable"
			a.Plan[0].Reason = "configured source unavailable"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Investigation{Status: "investigating", Plan: cloneInvestigationPlan(previous.Plan)}
			tc.edit(&a)
			err := prepareInvestigationPlan(&a, previous)
			if (err != nil) != tc.fail {
				t.Fatal(err)
			}
			if err == nil {
				a.Plan[0].Question = "changed"
				if previous.Plan[0].Question == "changed" {
					t.Fatal("alias")
				}
			}
		})
	}
}
func TestInvestigationPlanToolLifecycleUsesRealSourceIDs(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	tools := &auditTools{repo: repo, snap: snap, cache: map[string]string{}}
	a := Investigation{ID: "plan", Claim: "changed input", Plan: pendingPlan()}
	out, _ := tools.record(context.Background(), a)
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	a.Status = "supported"
	out, _ = tools.update(context.Background(), a)
	if out.Error == "" {
		t.Fatal("pending tasks resolved")
	}
	source, _ := tools.file(context.Background(), readArgs{Path: f.File, Start: 1, End: 2})
	if source.Error != "" {
		t.Fatal(source.Error)
	}
	a.ObservationIDs = []string{source.ObservationID}
	a.Evidence = []string{"source inspected"}
	for i := range a.Plan {
		a.Plan[i].Status = "checked"
		a.Plan[i].Reason = "conditional source fact"
		a.Plan[i].ObservationIDs = []string{source.ObservationID}
	}
	out, _ = tools.update(context.Background(), a)
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	a.Plan[0].ObservationIDs[0] = "forged"
	got := tools.investigations()
	if got[0].Plan[0].ObservationIDs[0] != source.ObservationID {
		t.Fatal("saved alias")
	}
	got[0].Plan[0].ObservationIDs[0] = "mutated"
	if tools.investigations()[0].Plan[0].ObservationIDs[0] != source.ObservationID {
		t.Fatal("returned alias")
	}
	a.Plan = nil
	a.ObservationIDs = nil
	out, _ = tools.update(context.Background(), a)
	if out.Error == "" {
		t.Fatal("inherited plan bypassed source ownership")
	}
	a.ObservationIDs = []string{source.ObservationID}
	out, _ = tools.update(context.Background(), a)
	if out.Error != "" {
		t.Fatal(out.Error)
	}
	if len(investigationPlanCoverage(tools.investigations())) != 0 {
		t.Fatal("completed recording gap")
	}
	a.Plan = cloneInvestigationPlan(tools.investigations()[0].Plan)
	a.Plan[0].ObservationIDs = []string{out.ObservationID}
	a.ObservationIDs = []string{out.ObservationID}
	out, _ = tools.update(context.Background(), a)
	if out.Error == "" {
		t.Fatal("process ID promoted")
	}
}
func TestInvestigationPlanCoverageAndRetainedBudget(t *testing.T) {
	if len(investigationPlanCoverage(nil)) == 0 || len(investigationPlanCoverage([]Investigation{{ID: "legacy"}})) == 0 {
		t.Fatal("missing plan hidden")
	}
	a := Investigation{ID: "risk", Plan: pendingPlan()}
	if len(investigationPlanCoverage([]Investigation{a})) != 4 {
		t.Fatal("pending gaps missing")
	}
	a.Plan = a.Plan[:1]
	a.Plan[0].Status = "checked"
	a.Plan[0].Reason = "fact"
	a.Plan[0].ObservationIDs = []string{"s"}
	a.Status = "supported"
	a.ObservationIDs = []string{"s"}
	if prepareInvestigationPlan(&a, Investigation{}) == nil {
		t.Fatal("missing aspect resolved")
	}
	previous := Investigation{Plan: pendingPlan()}
	for i := range previous.Plan {
		previous.Plan[i].Question = strings.Repeat("界", 300)
	}
	a = Investigation{Claim: strings.Repeat("x", 5000), Status: "investigating"}
	if prepareInvestigationPlan(&a, previous) == nil {
		t.Fatal("inherited payload bypassed budget")
	}
	var legacy Investigation
	if err := json.Unmarshal([]byte(`{"id":"old","status":"supported"}`), &legacy); err != nil || legacy.Status != "supported" || legacy.Plan != nil {
		t.Fatal(legacy, err)
	}
}

func hasPlanGap(notes []string) bool {
	return strings.Contains(strings.Join(notes, " "), "plan recording gap")
}
