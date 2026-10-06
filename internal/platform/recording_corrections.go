package platform

import (
	"context"
	"encoding/json"
	"fmt"
)

type recordingCorrection struct {
	FailedObservationID    string `json:"failed_observation_id"`
	CorrectedObservationID string `json:"corrected_observation_id"`
}
type recordingCorrectionsArgs struct {
	Corrections []recordingCorrection `json:"corrections"`
}

// An explicit correction retires pending work, never source provenance or the
// historical failure. A successful unrelated candidate cannot clear it.
func (t *auditTools) resolveRecordingErrors(ctx context.Context, args recordingCorrectionsArgs) (toolOutput, error) {
	return t.invoke("resolve_recording_errors", args, func() (toolOutput, error) {
		t.mu.Lock()
		defer t.mu.Unlock()
		if len(args.Corrections) < 1 || len(args.Corrections) > 8 {
			return toolOutput{}, fmt.Errorf("recording correction requires 1–8 pairs")
		}
		keys := []string{}
		seen := map[string]bool{}
		for _, pair := range args.Corrections {
			if pair.FailedObservationID == "" || len(pair.FailedObservationID) > 80 || pair.CorrectedObservationID == "" || len(pair.CorrectedObservationID) > 80 || seen[pair.FailedObservationID] {
				return toolOutput{}, fmt.Errorf("invalid or duplicate recording correction reference")
			}
			seen[pair.FailedObservationID] = true
			key, err := t.recordingCorrectionKeyLocked(pair)
			if err != nil {
				return toolOutput{}, err
			}
			keys = append(keys, key)
		}
		for _, key := range keys {
			delete(t.pending, key)
		}
		raw, _ := json.Marshal(args)
		return toolOutput{Text: string(raw)}, nil
	})
}

// Caller must hold t.mu. Navigation and mutation use the same eligibility
// boundary; projection alone never removes pending work.
func (t *auditTools) recordingCorrectionKeyLocked(pair recordingCorrection) (string, error) {
	failed, fi := t.recordingTrace(pair.FailedObservationID)
	corrected, ci := t.recordingTrace(pair.CorrectedObservationID)
	if fi < 0 || ci <= fi || failed.Error == "" || failed.Partial || corrected.Error != "" || corrected.Partial || !sameRecordingArtifact(failed, corrected) {
		return "", fmt.Errorf("correction requires a later successful recording of the same local artifact")
	}
	for _, tr := range []ToolTrace{failed, corrected} {
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || out.ObservationID != tr.ObservationID || out.RepositoryID != 0 || out.BaseSHA != t.snap.BaseSHA || out.HeadSHA != t.snap.HeadSHA || out.EvidenceEligible || tr.Stage != t.stage {
			return "", fmt.Errorf("correction references must belong to this audit recording context")
		}
	}
	var out toolOutput
	_ = json.Unmarshal([]byte(corrected.Output), &out)
	if out.Error != "" || out.Text == "" {
		return "", fmt.Errorf("correction has no accepted artifact")
	}
	key := ""
	for k, pending := range t.pending {
		if pending.ObservationID == failed.ObservationID {
			key = k
			break
		}
	}
	if key == "" {
		return "", fmt.Errorf("recording failure is not pending")
	}
	return key, nil
}
func (t *auditTools) recordingTrace(id string) (ToolTrace, int) {
	for i, tr := range t.trace {
		if tr.ObservationID == id {
			return tr, i
		}
	}
	return ToolTrace{}, -1
}
func recordingFamily(name string) string {
	switch name {
	case "record_hypothesis", "update_investigation":
		return "investigation"
	case "record_pr_context":
		return "pr_context"
	case "submit_finding":
		return "finding"
	}
	return ""
}
func sameRecordingArtifact(failed, corrected ToolTrace) bool {
	family := recordingFamily(failed.Name)
	if family == "" || family != recordingFamily(corrected.Name) {
		return false
	}
	if family == "investigation" || family == "pr_context" {
		var a, b Investigation
		if json.Unmarshal([]byte(failed.Arguments), &a) != nil || json.Unmarshal([]byte(corrected.Arguments), &b) != nil {
			return false
		}
		return a.ID != "" && a.ID == b.ID && a.Claim == b.Claim
	}
	var a, b Finding
	if json.Unmarshal([]byte(failed.Arguments), &a) != nil || json.Unmarshal([]byte(corrected.Arguments), &b) != nil {
		return false
	}
	return a.ID != "" && a.ID == b.ID && a.InvestigationID != "" && a.InvestigationID == b.InvestigationID && a.File == b.File && a.Type == b.Type && a.Title == b.Title && a.Description == b.Description && a.Trigger == b.Trigger
}

const recordingCorrectionGuidance = " If a recording or submit attempt fails and a later successful call corrects that same local artifact with different arguments, use resolve_recording_errors with the exact failed/corrected observation IDs to retire only that pending recording error. Keep the same local hypothesis/candidate id, and for findings the same investigation_id/file/type. Preserve the exact original claim, or finding title/description/trigger: changing the recorded statement is not a structural correction and cannot retire the earlier failure. This does not erase trace history, certify the claim or resolve missing source, pagination, plan or relationships. Do not link unrelated artifacts or hide actual unknowns."
