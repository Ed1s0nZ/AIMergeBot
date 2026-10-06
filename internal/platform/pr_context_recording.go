package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Only the existing PR context and deliberately supplied source links can change.
// Claim is an exact identity precondition, never a replacement claim.
type prContextRecording struct {
	ID                    string                  `json:"id"`
	Claim                 string                  `json:"claim"`
	PRContext             *PRInvestigationContext `json:"pr_context"`
	ObservationIDs        []string                `json:"observation_ids,omitempty"`
	CounterObservationIDs []string                `json:"counter_observation_ids,omitempty"`
}

func (t *auditTools) recordPRContext(_ context.Context, a prContextRecording) (toolOutput, error) {
	return t.invoke("record_pr_context", a, func() (toolOutput, error) {
		raw, _ := json.Marshal(a)
		if len(raw) > 8000 {
			return toolOutput{}, fmt.Errorf("PR context input exceeds 8000 UTF-8 bytes; shorten repeated facts")
		}
		if a.PRContext == nil {
			return toolOutput{}, fmt.Errorf("record_pr_context requires a complete pr_context; omission cannot erase a saved context")
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		current, ok := t.ledger[a.ID]
		if !ok || a.ID == "" {
			return toolOutput{}, unknownInvestigationError(t.ledger)
		}
		if a.Claim != current.Claim {
			return toolOutput{}, fmt.Errorf("record_pr_context claim must exactly match the saved investigation; this tool cannot revise a claim")
		}
		next := current
		next.PRContext = clonePRContext(a.PRContext)
		next.ObservationIDs = mergeContextSourceIDs(current.ObservationIDs, a.ObservationIDs)
		next.CounterObservationIDs = mergeContextSourceIDs(current.CounterObservationIDs, a.CounterObservationIDs)
		raw, _ = json.Marshal(next)
		if len(raw) > 8000 {
			return toolOutput{}, fmt.Errorf("merged investigation exceeds 8000 UTF-8 bytes; shorten PR context facts or explicitly revise the existing investigation")
		}
		if err := t.validateObservationIDs(next.ObservationIDs); err != nil {
			return toolOutput{}, err
		}
		if err := t.validateObservationIDs(next.CounterObservationIDs); err != nil {
			return toolOutput{}, err
		}
		if err := t.validatePrimaryContextSources(append(slices.Clone(next.ObservationIDs), next.CounterObservationIDs...)); err != nil {
			return toolOutput{}, err
		}
		if err := t.validatePRSnapshotLinks(next.PRContext); err != nil {
			return toolOutput{}, err
		}
		if err := validatePRContext(next); err != nil {
			return toolOutput{}, err
		}
		t.ledger[a.ID] = next
		return toolOutput{Text: string(raw), RecordingGaps: investigationRecordingGapsForBasis(next, metadataOnlyInvestigation(next, t.primaryMetadataOnlySourcesLocked()))}, nil
	})
}

// Caller holds t.mu. A new context record cannot launder later verification,
// duplicate trace IDs or failed/noneligible output into primary source links.
func (t *auditTools) validatePrimaryContextSources(ids []string) error {
	for _, id := range ids {
		count, valid := 0, false
		for _, tr := range t.trace {
			if tr.ObservationID != id {
				continue
			}
			count++
			var out toolOutput
			valid = tr.Error == "" && (tr.Stage == "" || tr.Stage == "primary") && isSourceTool(tr.Name) && json.Unmarshal([]byte(tr.Output), &out) == nil && out.EvidenceEligible && out.Error == "" && out.ObservationID == id && strings.TrimSpace(out.Text) != "" && observationAtSnapshot(out, t.snap)
		}
		if count != 1 || !valid {
			return fmt.Errorf("PR context source %q must identify exactly one successful eligible primary-stage read at the authorized fixed snapshot", id)
		}
	}
	return nil
}

func mergeContextSourceIDs(saved, added []string) []string {
	ids := slices.Clone(saved)
	for _, id := range added {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}
