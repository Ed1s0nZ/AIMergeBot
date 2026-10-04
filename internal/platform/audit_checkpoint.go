package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *Store) Checkpoint(ctx context.Context, id int64, result AuditResult, trace []ToolTrace) error {
	return s.CheckpointOwned(ctx, id, "", result, trace)
}
func (s *Store) CheckpointOwned(ctx context.Context, id int64, owner string, result AuditResult, trace []ToolTrace) error {
	if result.Findings == nil {
		result.Findings = []Finding{}
	}
	if result.CoverageNotes == nil {
		result.CoverageNotes = []string{}
	}
	if trace == nil {
		trace = []ToolTrace{}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tr, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_runs SET result_json=?,trace_json=? WHERE id=? AND status='running'`+workerFenceSQL, string(data), string(tr), id, owner)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (t *auditTools) checkpoint() {
	if t.progress == nil {
		return
	}
	t.progressMu.Lock()
	defer t.progressMu.Unlock()
	result := AuditResult{MetadataChanges: t.scope.metadataChanges(), Findings: t.acceptedFindings(), Investigations: t.investigations(), Summary: "Audit in progress; validated submissions checkpointed", CoverageNotes: []string{"Audit not yet complete"}}
	if t.supplementalResult != nil {
		result = *t.supplementalResult
	}
	t.mu.Lock()
	trace := append([]ToolTrace{}, t.trace...)
	t.mu.Unlock()
	if err := t.progress(result, trace); err != nil && !errors.Is(err, ErrConflict) {
		t.mu.Lock()
		t.progressError = fmt.Sprintf("Checkpoint persistence failed: %s", err)
		t.mu.Unlock()
	}
}
