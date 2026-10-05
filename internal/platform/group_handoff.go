package platform

import (
	"context"
	"encoding/json"
	"sort"
)

const groupHandoffBytes = 12 * 1024

type groupSourceLocator struct {
	ObservationID string          `json:"observation_id"`
	Tool          string          `json:"tool"`
	Arguments     json.RawMessage `json:"arguments"`
}
type groupFindingNote struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	File           string   `json:"file"`
	Side           string   `json:"side"`
	Line           int      `json:"line"`
	ObservationIDs []string `json:"observation_ids,omitempty"`
}
type groupHandoff struct {
	EvidenceEligible bool                 `json:"evidence_eligible"`
	Investigations   []Investigation      `json:"investigations"`
	Findings         []groupFindingNote   `json:"findings"`
	Sources          []groupSourceLocator `json:"source_locators"`
	Omitted          int                  `json:"omitted_items"`
}

// Prior claims are navigation data. No source output or canonical registry is shared.
func buildGroupHandoff(snap Snapshot, result AuditResult, trace []ToolTrace) (string, bool) {
	state := groupHandoff{Investigations: []Investigation{}, Findings: []groupFindingNote{}, Sources: []groupSourceLocator{}}
	fits := func() bool { raw, _ := json.Marshal(state); return len(raw) <= groupHandoffBytes-256 }
	ids := map[string]bool{}
	investigations := append([]Investigation{}, result.Investigations...)
	sort.SliceStable(investigations, func(i, j int) bool {
		return investigations[i].Status == "investigating" && investigations[j].Status != "investigating"
	})
	for _, item := range investigations {
		state.Investigations = append(state.Investigations, item)
		if !fits() {
			state.Investigations = state.Investigations[:len(state.Investigations)-1]
			state.Omitted++
			continue
		}
		for _, id := range append(append([]string{}, item.ObservationIDs...), item.CounterObservationIDs...) {
			ids[id] = true
		}
	}
	for _, f := range result.Findings {
		note := groupFindingNote{ID: f.ID, Title: f.Title, File: f.File, Side: f.Side, Line: f.Line, ObservationIDs: f.ObservationIDs}
		state.Findings = append(state.Findings, note)
		if !fits() {
			state.Findings = state.Findings[:len(state.Findings)-1]
			state.Omitted++
			continue
		}
		for _, id := range f.ObservationIDs {
			ids[id] = true
		}
	}
	located := map[string]bool{}
	for _, tr := range trace {
		if !ids[tr.ObservationID] || located[tr.ObservationID] || tr.Error != "" || !isSourceTool(tr.Name) || !json.Valid([]byte(tr.Arguments)) {
			continue
		}
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || out.Error != "" || out.Text == "" || !observationAtSnapshot(out, snap) {
			continue
		}
		state.Sources = append(state.Sources, groupSourceLocator{ObservationID: tr.ObservationID, Tool: tr.Name, Arguments: json.RawMessage(tr.Arguments)})
		if !fits() {
			state.Sources = state.Sources[:len(state.Sources)-1]
			continue
		}
		located[tr.ObservationID] = true
	}
	for id := range ids {
		if !located[id] {
			state.Omitted++
		}
	}
	raw, _ := json.Marshal(state)
	return string(raw), state.Omitted > 0
}

func groupHandoffAuthorization(ctx context.Context, snap Snapshot, sources map[int]ContextSource) string {
	if ctx.Err() != nil {
		return "Cross-group handoff unavailable: cancelled"
	}
	tools := &auditTools{snap: snap, contextSources: sources}
	for _, source := range contextPolicyItems(snap) {
		prepared, err := tools.contextSource(source.ProjectID)
		if err != nil || prepared.Repository == nil || (prepared.Authorize != nil && prepared.Authorize(ctx) != nil) {
			return "Cross-group handoff unavailable: context authorization"
		}
	}
	return ""
}
func (e *EinoAuditor) authorizedGroupHandoff(ctx context.Context, snap Snapshot, result AuditResult, trace []ToolTrace) (string, string) {
	if limitation := groupHandoffAuthorization(ctx, snap, e.ContextSources); limitation != "" {
		return "", limitation
	}
	raw, omitted := buildGroupHandoff(snap, result, trace)
	if limitation := groupHandoffAuthorization(ctx, snap, e.ContextSources); limitation != "" {
		return "", limitation
	}
	if omitted {
		return raw, "Cross-group handoff omitted items or source locations due to bounded input"
	}
	return raw, ""
}
