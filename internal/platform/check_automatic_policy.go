package platform

import "context"

func validateAutomaticCheckPolicy(ctx context.Context, q queryRower, snap Snapshot, actor int64, blocking bool) error {
	if snap.AuditPolicy == nil || snap.AuditPolicy.Workflow == nil || snap.AuditPolicy.Workflow.Checks == nil {
		return ErrConflict
	}
	captured := snap.AuditPolicy.Workflow
	rules := *captured.Checks
	if normalizeCheckRules(&rules) != nil || !rules.Enabled || rules.Publisher <= 0 || rules.Publisher != actor || (rules.Mode == "blocking") != blocking {
		return ErrConflict
	}
	current, err := readWorkflowPolicy(ctx, q, snap.ProjectID)
	if err != nil {
		return err
	}
	if current.Revision != captured.Revision || current.Checks == nil {
		return ErrConflict
	}
	active := *current.Checks
	if normalizeCheckRules(&active) != nil || active != rules {
		return ErrConflict
	}
	return nil
}

func (s *Store) QueueAutomaticRunCheck(ctx context.Context, id int64) error {
	snap, err := snapshotForRun(ctx, s.DB, id)
	if err != nil {
		return err
	}
	if snap.AuditPolicy == nil || snap.AuditPolicy.Workflow == nil || snap.AuditPolicy.Workflow.Checks == nil {
		return ErrConflict
	}
	rules := snap.AuditPolicy.Workflow.Checks
	return s.queueRunCheck(ctx, id, rules.Publisher, rules.Mode == "blocking", true)
}
