package platform

import "context"

// A durable cursor provides bounded, fair rescans even when a row is rejected.
func (s *Store) CollectRunChecks(ctx context.Context) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var cursor int64
	if err = tx.QueryRowContext(ctx, `SELECT last_run FROM platform_check_collector WHERE id=1`).Scan(&cursor); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id FROM platform_runs r JOIN platform_workflow_policies p ON p.project_id=r.project_id JOIN platform_projects project ON project.id=r.project_id AND project.enabled=1 JOIN platform_users actor ON actor.id=CASE WHEN json_valid(r.audit_policy_json) THEN json_extract(r.audit_policy_json,'$.workflow.checks.publisher') END AND actor.role='admin' AND actor.disabled=0 WHERE r.status IN ('succeeded','failed','incomplete','cancelled','skipped') AND CASE WHEN json_valid(r.audit_policy_json) THEN json_extract(r.audit_policy_json,'$.workflow.checks.enabled') END=1 AND p.revision=CASE WHEN json_valid(r.audit_policy_json) THEN json_extract(r.audit_policy_json,'$.workflow.revision') END AND NOT EXISTS(SELECT 1 FROM platform_runs n WHERE n.project_id=r.project_id AND n.mr_iid=r.mr_iid AND n.id>r.id) AND NOT EXISTS(SELECT 1 FROM platform_check_deliveries d WHERE d.run_id=r.id) ORDER BY CASE WHEN r.id>? THEN 0 ELSE 1 END,r.id LIMIT 100`, cursor)
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(ids) > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE platform_check_collector SET last_run=? WHERE id=1`, ids[len(ids)-1]); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	queued := 0
	var firstError error
	for _, id := range ids {
		if err = s.QueueAutomaticRunCheck(ctx, id); err != nil {
			if firstError == nil {
				firstError = err
			}
			continue
		}
		queued++
	}
	return queued, firstError
}
