package platform

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// QuotaError does not reveal other users' or projects' queue counts.
type QuotaError struct {
	Scope string
	Limit int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("audit quota reached: %s (limit %d)", e.Scope, e.Limit)
}

// BindQuotaSettings is safe before or during worker operation. Settings remain
// live service controls; they are not part of the immutable audit model policy.
func (s *Store) BindQuotaSettings(settings *SettingsService) { s.quotaSettings.Store(settings) }
func (s *Store) auditQuotas() AuditQuotas {
	q := AuditQuotas{}
	if settings := s.quotaSettings.Load(); settings != nil {
		q = settings.Snapshot().AuditQuotas
	}
	defaultAuditQuotas(&q)
	return q
}

func checkOutstanding(ctx context.Context, tx *sql.Tx, project int, actor int64, q AuditQuotas) error {
	for _, rule := range []struct {
		scope, filter string
		limit         int
		args          []any
	}{
		{"outstanding_global", "", q.OutstandingGlobal, nil},
		{"outstanding_project", " AND project_id=?", q.OutstandingProject, []any{project}},
		{"outstanding_user", " AND requested_by=?", q.OutstandingUser, []any{actor}},
	} {
		if rule.scope == "outstanding_user" && actor == 0 {
			continue
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_runs WHERE status IN ('pending','running')`+rule.filter, rule.args...).Scan(&count); err != nil {
			return err
		}
		if count >= rule.limit {
			return &QuotaError{Scope: rule.scope, Limit: rule.limit}
		}
	}
	return nil
}

// Correlated eligibility skips blocked projects/users without dropping their
// pending jobs. All counts and the running transition share a transaction.
func quotaClaimPredicate(q AuditQuotas) (string, []any) {
	return ` AND (SELECT COUNT(*) FROM platform_runs x WHERE x.status='running' AND x.project_id=r.project_id)<?
 AND (r.requested_by=0 OR (SELECT COUNT(*) FROM platform_runs x WHERE x.status='running' AND x.requested_by=r.requested_by)<?)
 AND (SELECT COUNT(*) FROM platform_runs x WHERE julianday(x.started_at)>=julianday('now','-24 hours'))<?
 AND (SELECT COUNT(*) FROM platform_runs x WHERE x.project_id=r.project_id AND julianday(x.started_at)>=julianday('now','-24 hours'))<?
 AND (r.requested_by=0 OR (SELECT COUNT(*) FROM platform_runs x WHERE x.requested_by=r.requested_by AND julianday(x.started_at)>=julianday('now','-24 hours'))<?)`,
		[]any{q.RunningProject, q.RunningUser, q.DailyGlobal, q.DailyProject, q.DailyUser}
}

type QueueWait struct {
	Reason     string `json:"reason"`
	EligibleAt string `json:"eligible_at,omitempty"`
}

// QueueWaitFor reports a current scheduling constraint, never a guarantee of
// execution time. Admission limits do not retroactively delete accepted jobs.
func (s *Store) QueueWaitFor(ctx context.Context, run Run) (*QueueWait, error) {
	if run.Status != "pending" {
		return nil, nil
	}
	if retry, err := time.Parse(time.RFC3339Nano, run.RetryAt); err == nil && retry.After(time.Now()) {
		return &QueueWait{Reason: "retry_delay", EligibleAt: run.RetryAt}, nil
	}
	q := s.auditQuotas()
	for _, rule := range []struct {
		reason, filter string
		limit          int
		args           []any
		daily          bool
	}{
		{"project_running", "status='running' AND project_id=?", q.RunningProject, []any{run.ProjectID}, false},
		{"user_running", "status='running' AND requested_by=?", q.RunningUser, []any{run.RequestedBy}, false},
		{"global_daily", "julianday(started_at)>=julianday('now','-24 hours')", q.DailyGlobal, nil, true},
		{"project_daily", "project_id=? AND julianday(started_at)>=julianday('now','-24 hours')", q.DailyProject, []any{run.ProjectID}, true},
		{"user_daily", "requested_by=? AND julianday(started_at)>=julianday('now','-24 hours')", q.DailyUser, []any{run.RequestedBy}, true},
	} {
		if run.RequestedBy == 0 && (rule.reason == "user_running" || rule.reason == "user_daily") {
			continue
		}
		var count int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_runs WHERE `+rule.filter, rule.args...).Scan(&count); err != nil {
			return nil, err
		}
		if count < rule.limit {
			continue
		}
		wait := &QueueWait{Reason: rule.reason}
		if rule.daily {
			args := append(append([]any{}, rule.args...), rule.limit-1)
			err := s.DB.QueryRowContext(ctx, `SELECT strftime('%Y-%m-%dT%H:%M:%fZ',started_at,'+24 hours') FROM platform_runs WHERE `+rule.filter+` ORDER BY julianday(started_at) DESC LIMIT 1 OFFSET ?`, args...).Scan(&wait.EligibleAt)
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
		}
		return wait, nil
	}
	return &QueueWait{Reason: "worker_available"}, nil
}
