package platform

import (
	"encoding/json"
	"fmt"
)

type InvestigationTask struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Question       string   `json:"question"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason,omitempty"`
	ObservationIDs []string `json:"observation_ids,omitempty"`
}

func cloneInvestigationPlan(plan []InvestigationTask) []InvestigationTask {
	if plan == nil {
		return nil
	}
	out := append([]InvestigationTask{}, plan...)
	for i := range out {
		out[i].ObservationIDs = append([]string{}, out[i].ObservationIDs...)
	}
	return out
}
func prepareInvestigationPlan(a *Investigation, previous Investigation) error {
	if a.Plan == nil {
		a.Plan = cloneInvestigationPlan(previous.Plan)
	}
	if len(a.Plan) > 8 {
		return fmt.Errorf("investigation plan exceeds 8 tasks")
	}
	allowed := map[string]bool{}
	for _, id := range append(append([]string{}, a.ObservationIDs...), a.CounterObservationIDs...) {
		allowed[id] = true
	}
	tasks := map[string]InvestigationTask{}
	for _, task := range a.Plan {
		known := task.Kind == "contract"
		for _, kind := range verificationKinds {
			known = known || task.Kind == kind
		}
		if !boundedFact(task.ID, 80) || !known || !boundedFact(task.Question, 400) || len(task.ObservationIDs) > 8 {
			return fmt.Errorf("invalid investigation plan task")
		}
		if _, exists := tasks[task.ID]; exists {
			return fmt.Errorf("duplicate investigation plan task")
		}
		if task.Reason != "" && !boundedFact(task.Reason, 400) {
			return fmt.Errorf("invalid investigation task reason")
		}
		switch task.Status {
		case "pending":
		case "unavailable":
			if task.Reason == "" {
				return fmt.Errorf("unavailable plan task needs a concrete reason")
			}
		case "checked":
			if task.Reason == "" || len(task.ObservationIDs) == 0 {
				return fmt.Errorf("checked plan task needs source IDs and a factual reason")
			}
		default:
			return fmt.Errorf("invalid investigation task status")
		}
		seen := map[string]bool{}
		for _, id := range task.ObservationIDs {
			if seen[id] {
				return fmt.Errorf("plan[].observation_ids contains a duplicate; include each source ID once per task")
			}
			if !allowed[id] {
				return fmt.Errorf("plan[].observation_ids must also appear in this investigation's top-level observation_ids or counter_observation_ids; add the already successful source ID to that list before referencing it in plan, or remove the unsupported citation. Never cite recording feedback as source evidence")
			}
			seen[id] = true
		}
		if (a.Status == "supported" || a.Status == "rejected") && task.Status != "checked" {
			return fmt.Errorf("cannot resolve investigation with unfinished plan task %q; keep investigating and inspect the needed source", task.ID)
		}
		tasks[task.ID] = task
	}
	if len(a.Plan) > 0 && (a.Status == "supported" || a.Status == "rejected") {
		kinds := map[string]bool{}
		for _, task := range a.Plan {
			kinds[task.Kind] = true
		}
		for _, kind := range verificationKinds {
			if !kinds[kind] {
				return fmt.Errorf("resolved plan is missing required aspect %s", kind)
			}
		}
	}
	for _, old := range previous.Plan {
		next, exists := tasks[old.ID]
		if !exists || next.Kind != old.Kind || next.Question != old.Question {
			return fmt.Errorf("existing plan tasks cannot be removed or redefined")
		}
	}
	// Inheriting a plan can expand the submitted payload. Reapply the same limit.
	raw, _ := json.Marshal(a)
	if len(raw) > 8000 {
		return fmt.Errorf("investigation including retained plan exceeds 8000 UTF-8 bytes")
	}
	a.Plan = cloneInvestigationPlan(a.Plan)
	return nil
}
func investigationPlanCoverage(items []Investigation) []string {
	if len(items) == 0 {
		return []string{"Investigation plan recording gap: no hypotheses or source-linked investigation plan recorded"}
	}
	notes := []string{}
	for _, item := range items {
		prefix := fmt.Sprintf("Investigation %s plan recording gap: ", item.ID)
		if len(item.Plan) == 0 {
			notes = append(notes, prefix+"no source-linked investigation plan recorded")
			continue
		}
		kinds := map[string]bool{}
		for _, task := range item.Plan {
			kinds[task.Kind] = true
		}
		for _, kind := range verificationKinds {
			if !kinds[kind] {
				notes = append(notes, prefix+"required aspect "+kind+" is not planned")
			}
		}
		for _, task := range item.Plan {
			if task.Status != "checked" {
				notes = append(notes, prefix+fmt.Sprintf("task %s (%s) remains %s: %s", task.ID, task.Kind, task.Status, task.Question))
			}
		}
	}
	return notes
}

const investigationPlanGuidance = " For every PR/group, record at least one changed-behavior inspection with a concise plan after the first relevant source read and before expanding source searches. This includes a guard improvement, safe refactor or compatible/no-finding conclusion: record that actual claim, not an invented vulnerability. A plan contains up to8 tasks with local id, kind(input_control|pr_causality|guards|outcome|contract), question, status(pending|checked|unavailable), reason and observation_ids. Identify relevant input/caller, BASE/HEAD causal comparison, protections/counterevidence, actual contract and harmful outcome; use contract tasks for relevant downstream sources. Initially use pending tasks without source IDs. Update to checked only with a factual reason and exact successful source IDs linked to this investigation; unavailable needs a concrete reason and remains unresolved. Do not omit or redefine previously recorded tasks to obtain completion. Omitted plan on update inherits the saved plan. supported/rejected investigations with a plan require all four core kinds(input_control,pr_causality,guards,outcome) present and every task checked; necessary missing context stays investigating and coverage_notes. Plan task completion means inspection/recording, never semantic proof. Plan omission never certifies coverage. Keep serialized investigation including plan <=8000 UTF-8 bytes; omit repeated source text, not important uncertainty."
