package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"sort"
	"strings"
)

func (t *auditTools) investigations() []Investigation {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []Investigation{}
	for _, v := range t.ledger {
		v.Plan = cloneInvestigationPlan(v.Plan)
		v.PRContext = clonePRContext(v.PRContext)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (t *auditTools) record(ctx context.Context, a Investigation) (toolOutput, error) {
	return t.ledgerChange("record_hypothesis", a)
}
func (t *auditTools) update(ctx context.Context, a Investigation) (toolOutput, error) {
	return t.ledgerChange("update_investigation", a)
}
func (t *auditTools) ledgerChange(name string, a Investigation) (toolOutput, error) {
	return t.invoke(name, a, func() (toolOutput, error) {
		raw, _ := json.Marshal(a)
		if len(raw) > 8000 {
			return toolOutput{}, fmt.Errorf("investigation JSON is %d UTF-8 bytes; maximum is 8000 bytes. Shorten repeated evidence text and PR facts; retain a few core source-linked statements and unresolved next_steps. Do not repeat the oversized payload", len(raw))
		}
		if strings.TrimSpace(a.Claim) == "" {
			return toolOutput{}, fmt.Errorf("investigation claim must be nonempty")
		}
		if len(a.ID) > 80 {
			return toolOutput{}, fmt.Errorf("investigation ID exceeds 80 bytes; copy the returned existing ID")
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		// Omission on update retains the recorded context, but never its source validation.
		if name == "update_investigation" && a.PRContext == nil {
			a.PRContext = clonePRContext(t.ledger[a.ID].PRContext)
		}
		if err := t.validateObservationIDs(a.ObservationIDs); err != nil {
			return toolOutput{}, err
		}
		if err := t.validateObservationIDs(a.CounterObservationIDs); err != nil {
			return toolOutput{}, err
		}
		if err := t.validatePRSnapshotLinks(a.PRContext); err != nil {
			return toolOutput{}, err
		}
		if err := validatePRContext(a); err != nil {
			return toolOutput{}, err
		}
		if t.ledger == nil {
			t.ledger = map[string]Investigation{}
		}
		if name == "record_hypothesis" {
			if len(t.ledger) >= 30 {
				return toolOutput{}, fmt.Errorf("investigation budget exhausted")
			}
			if a.ID == "" {
				a.ID = fmt.Sprintf("hypothesis-%d", len(t.ledger)+1)
			}
			if _, exists := t.ledger[a.ID]; exists {
				return toolOutput{}, fmt.Errorf("hypothesis already exists")
			}
			a.Status = "investigating"
		} else {
			if _, exists := t.ledger[a.ID]; !exists {
				return toolOutput{}, fmt.Errorf("unknown hypothesis")
			}
			if a.Status != "investigating" && a.Status != "supported" && a.Status != "rejected" {
				return toolOutput{}, fmt.Errorf("invalid investigation status")
			}
			if a.Status == "supported" && (len(a.Evidence) == 0 || len(a.ObservationIDs) == 0) || a.Status == "rejected" && (len(a.Counterevidence) == 0 || len(a.CounterObservationIDs) == 0) {
				return toolOutput{}, fmt.Errorf("resolved hypothesis needs evidence/counterevidence and successful source observation IDs")
			}
		}
		if err := prepareInvestigationPlan(&a, t.ledger[a.ID]); err != nil {
			return toolOutput{}, err
		}
		a.PRContext = clonePRContext(a.PRContext)
		t.ledger[a.ID] = a
		raw, _ = json.Marshal(a)
		return toolOutput{Text: string(raw), RecordingGaps: investigationRecordingGaps(a)}, nil
	})
}
func (t *auditTools) submit(ctx context.Context, a Finding) (toolOutput, error) {
	return t.invoke("submit_finding", a, func() (toolOutput, error) {
		accepted, e := t.acceptFinding(ctx, a)
		if e != nil {
			return toolOutput{}, e
		}
		raw, _ := json.Marshal(accepted)
		return toolOutput{Text: string(raw), RecordingGaps: prRecordingGaps(accepted.PRContext, accepted.AnchorType == "git_metadata" && accepted.Metadata != nil)}, nil
	})
}
func (t *auditTools) register() ([]tool.BaseTool, error) {
	out := []tool.BaseTool{}
	// Each handler is typed so Eino generates the structured schema; never accept raw shell commands.
	add := func(v tool.BaseTool, e error) error {
		if e != nil {
			return e
		}
		out = append(out, v)
		return nil
	}
	if e := add(utils.InferTool("get_risk_checklist", "Get bounded investigation and counterevidence questions for one relevant risk category. Guidance only, evidence_eligible=false; cannot support a finding or verification verdict.", t.riskChecklist)); e != nil {
		return nil, e
	}
	if len(contextPolicyItems(t.snap)) > 0 {
		if e := add(utils.InferTool("list_repositories", "List administrator-authorized fixed context repository IDs/SHA/availability. Existing read_file/search_code address the primary PR; use scoped context tools for listed IDs only. Enumeration is not source evidence.", t.contextRepositories)); e != nil {
			return nil, e
		}
		if e := add(utils.InferTool("read_repository_file", "Read numbered lines from one authorized context repository at its fixed SHA, max 200 lines. repository_id is a listed context project ID, not an arbitrary destination. Context facts cannot replace primary changed-line evidence.", t.contextFile)); e != nil {
			return nil, e
		}
		if e := add(utils.InferTool("read_repository_files", "Read 1–8 related file ranges from one authorized context repository at its fixed SHA. Each range uses path/start/end, max 200 lines; base must be false. Aggregate output bounded to 16000 bytes. Context facts cannot replace primary PR change evidence; remaining or more requires follow-up reads.", t.contextBatch)); e != nil {
			return nil, e
		}
		if e := add(utils.InferTool("search_repository_code", "Search the entire selected fixed context repository, literal/regex/path/extension/case filters, next_cursor pagination. Results are lexical candidates, not semantic call proof.", t.contextSearch)); e != nil {
			return nil, e
		}
		if e := add(utils.InferTool("list_repository_directory", "List bounded directory tree in the selected fixed context repository, depth 1–20 and cursor pagination. Enumeration is not source evidence.", t.contextDirectory)); e != nil {
			return nil, e
		}
	}
	if e := add(utils.InferTool("read_file", "Read numbered lines at pinned head/base. start/end max 200 lines. remaining is unread file content, more is truncation.", t.file)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("get_change_metadata", "Read canonical Git metadata for an included changed path. Use the entire text as metadata finding evidence, anchor_type git_metadata and line 0. File modes/object IDs are facts, not proof of a vulnerability.", t.changeMetadata)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("list_files", "List head file names, page size 100. Enumeration is not source evidence; evidence_eligible=false.", t.list)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("search_code", "Search all pinned text files, literal or extended regex, path prefix, extension and case filtering. Continue next_cursor when more. Lexical matches are not semantic references.", t.search)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("list_directory", "Explore directory tree, path/depth (1–20), base/head and cursor.", t.directory)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("read_files", "Read 1–8 related files with individual base/start/end; aggregate output bounded.", t.batch)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("get_diff", "Inspect pinned base/head diff, optionally literal path; cursor paginated.", t.diff)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("compare_files", "Compare base old_path (defaults path) and head path; native Git only. For additions/deletions use get_diff.", t.compare)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("get_history", "Read bounded commit history and patches reachable from base/head; optional path, limit<=100, skip commits and output cursor. Shallow boundary is disclosed.", t.history)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("git_blame", "Inspect origin of path lines start/end, base/head; shallow boundaries possible.", t.blame)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("search_history", "Search changes in occurrence count of literal query in reachable history, optionally path; bounded history, limit/cursor.", t.historySearch)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("record_hypothesis", "Record concise factual claim, evidence, counterevidence, observation_ids, counter_observation_ids and next_steps; not private reasoning. Always creates status investigating regardless of input status; returns generated id. Resolve later with update_investigation only after source-linked inspection, or preserve actual unknowns. Entire serialized investigation max8000 UTF-8 bytes: keep only concise core facts, use observation IDs instead of repeating source text.", t.record)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("update_investigation", "Update existing id, claim and status investigating/supported/rejected with evidence/observation_ids for supported; rejected REQUIRES counterevidence AND counter_observation_ids. Copy only IDs whose output evidence_eligible=true; error eligible_observation_ids is guidance, not automatic linkage. Omitted plan and pr_context retain saved records; retained source IDs must still belong to this update. Entire serialized update including retained records max8000 UTF-8 bytes; keep a few core facts, not full repeated source blocks. Does not prove exploitability.", t.update)); e != nil {
		return nil, e
	}
	if e := add(utils.InferTool("submit_finding", "Validate proposed finding against changed base/head lines or verified Git metadata and exact snapshot evidence; matching evidence does not establish runtime verification.", t.submit)); e != nil {
		return nil, e
	}
	return out, nil
}
