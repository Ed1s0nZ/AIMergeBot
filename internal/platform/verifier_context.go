package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cloudwego/eino/callbacks"
	em "github.com/cloudwego/eino/components/model"
)

func claimObservationPrefix(item Investigation) string {
	// Length framing keeps artifact/claim boundaries distinct even for unusual
	// historical identifiers; no model-controlled identifier enters a source ID.
	key := fmt.Sprintf("%d:%s%d:%s", len(item.ID), item.ID, len(item.Claim), item.Claim)
	digest := sha256.Sum256([]byte(key))
	return fmt.Sprintf("claim-%x-observation", digest)
}

func (e *EinoAuditor) verifyAuditJudgments(ctx context.Context, result *AuditResult, parent *auditTools, model em.ToolCallingChatModel) {
	pool := newVerificationBudget(ctx)
	defer pool.cancel()
	e.verifyFindingsWithBudget(ctx, result, parent, model, pool)
	e.verifyInvestigationClaims(ctx, result, parent, model, pool)
}

func verifierSourceProgress(parent, fresh *auditTools) func(AuditResult, []ToolTrace) error {
	return func(_ AuditResult, traces []ToolTrace) error {
		fresh.mu.Lock()
		unavailable := fresh.repositoryUnavailable
		fresh.mu.Unlock()
		parent.mu.Lock()
		parent.repositoryUnavailable = parent.repositoryUnavailable || unavailable
		seen := map[string]bool{}
		for _, tr := range parent.trace {
			seen[tr.ObservationID] = true
		}
		for _, tr := range traces {
			if tr.ObservationID != "" && !seen[tr.ObservationID] {
				parent.trace = append(parent.trace, tr)
				seen[tr.ObservationID] = true
			}
		}
		parent.mu.Unlock()
		parent.checkpoint()
		return nil
	}
}

func verifierModelCallbacks(parent *auditTools, stage, model string) callbacks.Handler {
	return callbacks.NewHandlerBuilder().OnStartFn(modelStartCallback(parent, stage, model)).OnErrorFn(modelFailureCallback(parent, stage, model)).OnEndFn(func(c context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		if data, ok := output.(*em.CallbackOutput); ok {
			tr := ToolTrace{Name: "model", Stage: stage, Arguments: model}
			if data.TokenUsage != nil {
				tr.TotalTokens = data.TokenUsage.TotalTokens
				tr.PromptTokens = data.TokenUsage.PromptTokens
				tr.CompletionTokens = data.TokenUsage.CompletionTokens
				tr.UsageReported = true
			}
			recordModelTrace(parent, tr)
		}
		return c
	}).Build()
}

func recordClaimResponseFailure(parent *auditTools, raw, code string) {
	parent.mu.Lock()
	defer parent.mu.Unlock()
	parent.trace = append(parent.trace, ToolTrace{Name: "model_response", Stage: claimVerificationStage, Error: code, Output: responseDiagnostic(raw, responseError(code))})
}

func claimNavigationPaths(scope DiffScope) []string {
	seen := map[string]bool{}
	for _, path := range scope.Included {
		seen[path] = true
	}
	for path := range scope.Added {
		seen[path] = true
	}
	for path := range scope.Removed {
		seen[path] = true
	}
	for path := range scope.Metadata {
		seen[path] = true
	}
	paths := []string{}
	for path := range seen {
		if validPath(path) && len(path) <= 512 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

type claimSourceLocator struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}
type claimReviewNavigation struct {
	ChangedPaths []string             `json:"changed_paths"`
	Sources      []claimSourceLocator `json:"source_locators"`
	Omitted      bool                 `json:"omitted"`
}

// Locators are untrusted navigation only. No primary judgment, source text or
// primary observation ID enters the independent context.
func buildClaimReviewNavigation(parent *auditTools, item Investigation) claimReviewNavigation {
	n := claimReviewNavigation{ChangedPaths: claimNavigationPaths(parent.scope), Sources: []claimSourceLocator{}}
	if len(n.ChangedPaths) > 20 {
		n.ChangedPaths = n.ChangedPaths[:20]
		n.Omitted = true
	}
	ids := map[string]bool{}
	located := map[string]bool{}
	locators := map[string]bool{}
	for _, id := range append(append([]string{}, item.ObservationIDs...), item.CounterObservationIDs...) {
		ids[id] = true
	}
	parent.mu.Lock()
	defer parent.mu.Unlock()
	for _, tr := range parent.trace {
		if !ids[tr.ObservationID] || tr.Error != "" || (tr.Stage != "" && tr.Stage != "primary") {
			continue
		}
		switch tr.Name {
		case "read_file", "read_files", "read_repository_file", "read_repository_files", "get_diff", "compare_files", "get_change_metadata":
		default:
			continue
		}
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || !out.EvidenceEligible || !observationAtSnapshot(out, parent.snap) {
			continue
		}
		key := tr.Name + "\x00" + tr.Arguments
		if locators[key] {
			located[tr.ObservationID] = true
			continue
		}
		if len(n.Sources) >= 12 || len(tr.Arguments) > 1024 || !json.Valid([]byte(tr.Arguments)) {
			n.Omitted = true
			continue
		}
		n.Sources = append(n.Sources, claimSourceLocator{Tool: tr.Name, Arguments: json.RawMessage(tr.Arguments)})
		locators[key], located[tr.ObservationID] = true, true
	}
	for id := range ids {
		if !located[id] {
			n.Omitted = true
		}
	}
	return n
}
