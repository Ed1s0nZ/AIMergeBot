package platform

import (
	"context"
	"fmt"
)

type riskChecklistArgs struct {
	Category string `json:"category" jsonschema:"description=access_control, tenant_isolation, business_state, concurrency, or injection"`
}

// Independently authored investigation guidance, never repository evidence.
var riskChecklists = map[string]string{
	"access_control":   "Identify the changed operation and its reachable caller. Compare BASE/HEAD authentication, ownership, role and resource checks. Inspect middleware and store-level protections before claiming removal causes a bypass. Check whether the actor can choose the target resource. Counterexample: an equivalent guard remains in a caller or callee. Unknown authorization context means unresolved reachability, not absent protection.",
	"tenant_isolation": "Trace tenant identity from the caller to reads, writes, cache keys and background work affected by this PR. Compare BASE/HEAD scoping and identity propagation. Check whether tenant identity is server-derived or caller-controlled and inspect both sides of any repository contract. Counterexample: mandatory tenant filtering or a globally unique authorization-bound identifier preserves isolation. Matching names alone do not establish a cross-project contract.",
	"business_state":   "Identify the state transition changed by this PR, permitted actors, preconditions and invariants. Compare validation order, transaction boundaries, retries and idempotency before/after. Establish a reachable harmful consequence such as duplicate settlement or forbidden transition. Counterexample: a unique constraint, atomic conditional update or upstream invariant still enforces the rule. Missing deployment assumptions remain explicit.",
	"concurrency":      "Identify shared state and overlapping operations affected by the change. Compare BASE/HEAD atomicity, locking, transaction isolation and retry behavior. Establish a concrete interleaving and harmful outcome; Counterexample: database constraints or caller serialization may prevent that interleaving. A read followed by a write is not automatically a race. Static interleavings are hypotheses, not reproduced failures.",
	"injection":        "Trace controllable input through the changed transformation to the dangerous operation. Compare BASE/HEAD parameter binding, argument boundaries, escaping, validation and execution context. Inspect driver/API semantics and relevant callers. Counterexample: binding or context-appropriate encoding remains effective. A dangerous-looking API or string concatenation is not sufficient without a reachable input and harmful consequence.",
}

const prInvestigationGuidance = " Start from this PR's changed behavior, not a whole-project vulnerability inventory. Before expanding searches, identify the affected entry point or contract and compare BASE versus HEAD protections and consequences. For important PR hypotheses, populate investigation pr_context with change_summary, before, after, source-linked entry_points, guards, impact and checked counterexamples, plus relationships and unresolved_edges. Link before_observation_ids and after_observation_ids to the actual BASE/HEAD reads. For each relationship give from/to/relation and source observation_ids; cited requires inspected connection evidence such as dispatch, argument binding or a cross-project contract, never just matching names. Use inferred for a source-linked candidate edge and retain unproven connections in unresolved_edges. Do not label inferred paths as proven consequences. Facts use statement and observation_ids from this investigation; unknown entry points belong in unresolved_edges, never invented source facts. Record sourced entry/contract facts and unresolved edges in the existing investigation ledger; carry exact observation IDs and next_steps across the investigation. Use get_risk_checklist only for relevant risk categories, not all categories by default. Checklists are navigation guidance, never source evidence or coverage. Verify the suggested counterexamples against actual code. Unchanged historical defects without a causal relation to this PR are not new PR findings. Cross-project names or matching snippets alone never prove a contract or call edge. Do not assume BASE behavior was correct. First identify the source-supported intended contract or invariant; choose an explicit input and compare observable BASE and HEAD outcomes through the actual fixed downstream contract. Explain which intended invariant HEAD violates, not just how much a value changed. A smaller/larger/different result than BASE is not by itself harmful; HEAD may fix an existing incompatibility or incorrect transformation. If intended behavior is unknown, record that uncertainty instead of inventing a regression. Record concise contract/outcome facts, not private reasoning."

func (t *auditTools) riskChecklist(ctx context.Context, a riskChecklistArgs) (toolOutput, error) {
	return t.invoke("get_risk_checklist", a, func() (toolOutput, error) {
		if err := ctx.Err(); err != nil {
			return toolOutput{}, err
		}
		text, ok := riskChecklists[a.Category]
		if !ok {
			return toolOutput{}, fmt.Errorf("unknown risk category; use access_control, tenant_isolation, business_state, concurrency, or injection")
		}
		return toolOutput{Text: "Investigation guidance only; not source evidence.\n" + text}, nil
	})
}
