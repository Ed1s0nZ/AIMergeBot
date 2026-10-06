package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eo "github.com/cloudwego/eino-ext/components/model/openai"
)

type AuditGroupProgress struct {
	StopReason     string   `json:"stop_reason,omitempty"`
	PriorityWeight int      `json:"priority_weight,omitempty"`
	ID             string   `json:"id"`
	Files          []string `json:"files"`
	Status         string   `json:"status"`
}

// mergeAuditGroup operates on a deep copy: checkpoints cannot mutate a live
// group's canonical registry or rename its investigation references in place.
func mergeAuditGroup(base AuditResult, group AuditResult, id string) AuditResult {
	raw, _ := json.Marshal(group)
	var copy AuditResult
	_ = json.Unmarshal(raw, &copy)
	for i := range copy.Investigations {
		copy.Investigations[i].ID = id + "-" + copy.Investigations[i].ID
	}
	seen := map[string]bool{}
	for _, f := range base.Findings {
		seen[f.ID] = true
	}
	for _, f := range copy.Findings {
		if f.InvestigationID != "" {
			f.InvestigationID = id + "-" + f.InvestigationID
		}
		if !seen[f.ID] {
			base.Findings = append(base.Findings, f)
			seen[f.ID] = true
		}
	}
	base.Investigations = append(base.Investigations, copy.Investigations...)
	metadataSeen := map[string]bool{}
	for _, metadata := range base.MetadataChanges {
		metadataSeen[metadata.canonical()] = true
	}
	for _, metadata := range copy.MetadataChanges {
		key := metadata.canonical()
		if !metadataSeen[key] {
			base.MetadataChanges = append(base.MetadataChanges, metadata)
			metadataSeen[key] = true
		}
	}
	base.CoverageNotes = append(base.CoverageNotes, copy.CoverageNotes...)
	return base
}

func (e *EinoAuditor) AuditGroups(ctx context.Context, snap Snapshot, plan AuditPlan) (AuditResult, []ToolTrace, error) {
	ctx = withModelBudget(ctx, e.Config.MaxTokens)
	result := AuditResult{Findings: []Finding{}, CoverageNotes: append([]string{}, plan.Notes...), ExcludedFiles: append([]string{}, plan.Excluded...), Summary: "Grouped static audit"}
	for _, g := range plan.Groups {
		result.AuditGroups = append(result.AuditGroups, AuditGroupProgress{PriorityWeight: groupPriority(g), ID: g.ID, Files: g.Files, Status: "unprocessed"})
	}
	trace := []ToolTrace{}
	manifestPaths := []string{}
	manifestSeen := map[string]bool{}
	for _, g := range plan.Groups {
		for _, file := range g.Files {
			if !manifestSeen[file] {
				manifestPaths = append(manifestPaths, file)
				manifestSeen[file] = true
			}
		}
	}
	manifest, _ := json.Marshal(map[string]any{"files": manifestPaths, "excluded": plan.Excluded, "coverage_notes": plan.Notes, "omitted_files": plan.OmittedFiles})
	if len(manifest) > 16*1024 {
		manifest = []byte(`{"incomplete":true,"reason":"changed-path manifest exceeds budget"}`)
		result.CoverageNotes = append(result.CoverageNotes, "Changed-path manifest exceeds budget")
	}
	remaining := e.Config.MaxToolCalls
	if remaining <= 0 {
		remaining = 80
	}
	primaryCtx := ctx
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		primaryCtx, cancel = context.WithDeadline(ctx, deadline.Add(-20*time.Second))
	}
	defer cancel()
	summaries := []string{}
	var repositoryFailure error
	for i, g := range plan.Groups {
		if repositoryFailure != nil || primaryCtx.Err() != nil || remaining <= 0 {
			result.CoverageNotes = append(result.CoverageNotes, "Unprocessed audit group: "+g.ID)
			continue
		}
		result.AuditGroups[i].Status = "running"
		child := *e
		currentGroup := g
		child.Config.CurrentGroup = &currentGroup
		child.Config.PrimaryOnly = true
		child.Config.ObservationPrefix = g.ID + "-observation"
		child.Config.Manifest = string(manifest)
		child.Config.PriorGroupNotes = ""
		if i > 0 {
			notes, limitation := e.authorizedGroupHandoff(primaryCtx, snap, result, trace)
			child.Config.PriorGroupNotes = notes
			if limitation != "" {
				result.CoverageNotes = append(result.CoverageNotes, limitation)
			}
		}
		child.Config.MaxToolCalls = priorityCallBudget(remaining, plan.Groups[i:])
		if child.Config.MaxToolCalls < 1 {
			child.Config.MaxToolCalls = 1
		}
		completed := result
		completedTrace := append([]ToolTrace{}, trace...)
		child.Config.Progress = func(part AuditResult, current []ToolTrace) error {
			if e.Config.Progress == nil {
				return nil
			}
			merged := mergeAuditGroup(completed, part, g.ID)
			return e.Config.Progress(merged, append(append([]ToolTrace{}, completedTrace...), current...))
		}
		groupCtx := primaryCtx
		groupCancel := func() {}
		if deadline, ok := primaryCtx.Deadline(); ok {
			groupCtx, groupCancel = context.WithTimeout(primaryCtx, priorityTimeBudget(time.Until(deadline), plan.Groups[i:]))
		}
		part, current, err := child.Audit(groupCtx, snap, g.Scope)
		groupCancel()
		calls := 0
		for _, tr := range current {
			if tr.Name != "model" {
				calls++
			}
		}
		remaining -= calls
		result = mergeAuditGroup(result, part, g.ID)
		trace = append(trace, current...)
		if err != nil {
			if errors.Is(err, ErrRepositoryUnavailable) {
				repositoryFailure = err
			}
			result.AuditGroups[i].Status = "failed"
			result.AuditGroups[i].StopReason = auditStopReason(err)
			result.CoverageNotes = append(result.CoverageNotes, "Audit group failed: "+g.ID)
		} else {
			result.AuditGroups[i].Status = "completed"
			summaries = append(summaries, g.ID+": "+part.Summary)
		}
		if e.Config.Progress != nil {
			if err := e.Config.Progress(result, trace); err != nil {
				return result, trace, fmt.Errorf("group checkpoint failed: %w", err)
			}
		}
	}
	result.Summary = "Grouped static audit\n" + strings.Join(summaries, "\n")
	if repositoryFailure != nil {
		result.Summary = "Grouped audit stopped because fixed repository execution is unavailable; prior results retained\n" + strings.Join(summaries, "\n")
	}
	// Supplemental stages use the full planned anchor scope and preserved fresh
	// provenance, once for all accepted findings under the original task deadline.
	scope := DiffScope{Added: map[string]map[int]bool{}, Removed: map[string]map[int]bool{}, Metadata: map[string]GitChangeMetadata{}}
	for _, g := range plan.Groups {
		mergeScopeAnchors(&scope, g.Scope)
	}
	tools := &auditTools{contextSources: e.ContextSources, repo: e.Repository, snap: snap, scope: scope, cache: map[string]string{}, trace: trace, progress: e.Config.Progress, maxCalls: e.Config.MaxToolCalls}
	tools.sequenceCheckpoint(result)
	if ctx.Err() == nil && repositoryFailure == nil {
		cfg := e.Config
		tokens := 4096
		model, err := eo.NewChatModel(ctx, &eo.ChatModelConfig{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model, Temperature: &cfg.Temperature, MaxTokens: &tokens, HTTPClient: upstreamHTTPClient("model"), ResponseFormat: &eo.ChatCompletionResponseFormat{Type: eo.ChatCompletionResponseFormatTypeJSONObject}})
		if err != nil {
			result.CoverageNotes = append(result.CoverageNotes, "Grouped supplemental model unavailable")
		} else {
			registered, err := tools.register()
			if err != nil {
				result.CoverageNotes = append(result.CoverageNotes, "Grouped supplemental tools unavailable")
			} else {
				e.synthesizeGroups(ctx, snap, &result, tools, string(manifest), remaining, budgetModel(model))
				tools.sequenceCheckpoint(result)
				e.supplement(ctx, snap, &result, tools, registered, budgetModel(model), "")
			}
		}
	}
	result.CoverageNotes = uniqueCoverageNotes(result.CoverageNotes)
	tools.sequenceCheckpoint(result)
	return result, tools.trace, repositoryFailure
}
