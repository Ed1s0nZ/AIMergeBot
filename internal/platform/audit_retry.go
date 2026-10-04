package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	modelopenai "github.com/meguminnnnnnnnn/go-openai"
	"github.com/xanzy/go-gitlab"
	"net"
	"time"
)

const maxAutomaticRetries = 2

func retryableError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	status := 0
	var api *modelopenai.APIError
	var request *modelopenai.RequestError
	var git *gitlab.ErrorResponse
	var network net.Error
	if errors.As(err, &api) {
		status = api.HTTPStatusCode
	} else if errors.As(err, &request) {
		status = request.HTTPStatusCode
	} else if errors.As(err, &git) && git.Response != nil {
		status = git.Response.StatusCode
	}
	if status != 0 {
		return status == 429 || status >= 500 && status <= 599
	}
	return errors.As(err, &network) && (network.Timeout() || network.Temporary())
}

// FailAndRetry atomically records the failed attempt and creates its delayed child.
func (s *Store) FailAndRetry(ctx context.Context, id int64, message string, result AuditResult, trace []ToolTrace, delay time.Duration) (int64, error) {
	return s.FailAndRetryOwned(ctx, id, "", message, result, trace, delay)
}
func (s *Store) FailAndRetryOwned(ctx context.Context, id int64, owner, message string, result AuditResult, trace []ToolTrace, delay time.Duration) (int64, error) {
	return s.failAndRetry(ctx, id, owner, message, result, trace, delay, false)
}
func (s *Store) recoverAttempt(ctx context.Context, id int64) (int64, error) {
	return s.failAndRetry(ctx, id, "", "worker lease expired or service interrupted; checkpoint retained", AuditResult{}, nil, 0, true)
}
func (s *Store) failAndRetry(ctx context.Context, id int64, owner, message string, result AuditResult, trace []ToolTrace, delay time.Duration, recovery bool) (int64, error) {
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
		return 0, err
	}
	tr, err := json.Marshal(trace)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var res sql.Result
	if recovery {
		var attempt int
		if err = tx.QueryRowContext(ctx, `SELECT retry_attempt FROM platform_runs WHERE id=? AND status='running'`+expiredWorkerSQL, id).Scan(&attempt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, ErrConflict
			}
			return 0, err
		}
		delay = retryDelay(attempt)
		res, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error=?,finished_at=? WHERE id=? AND status='running'`+expiredWorkerSQL, message, now(), id)
	} else {
		res, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error=?,result_json=?,trace_json=?,finished_at=? WHERE id=? AND status='running'`+workerFenceSQL, message, string(data), string(tr), now(), id, owner)
	}
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, ErrConflict
	}
	if recovery {
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(0,'run.recovered',?,?)`, fmt.Sprint(id), now()); err != nil {
			return 0, err
		}
	}
	retryAt := ""
	if delay > 0 {
		retryAt = time.Now().UTC().Add(delay).Format(time.RFC3339Nano)
	}
	res, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version,policy_digest,audit_policy_json,retry_parent_id,retry_attempt,retry_at) SELECT project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,'pending',?,requested_by,policy_version,policy_digest,audit_policy_json,id,retry_attempt+1,? FROM platform_runs WHERE id=? AND retry_attempt<? AND policy_version=?`, now(), retryAt, id, maxAutomaticRetries, PolicyVersion)
	if err != nil {
		return 0, err
	}
	n, err = res.RowsAffected()
	if err != nil {
		return 0, err
	}
	child := int64(0)
	if n == 1 {
		child, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(0,'run.retry_scheduled',?,?)`, fmt.Sprintf("parent=%d child=%d", id, child), now()); err != nil {
			return 0, err
		}
	}
	return child, tx.Commit()
}
func retryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 5 * time.Second
	}
	return 20 * time.Second
}
func (r *Runner) failTransient(id int64, err error, result AuditResult, trace []ToolTrace) bool {
	if r.workerStopped() {
		return true
	}
	if !retryableError(err) {
		return false
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	run, e := r.Store.Run(ctx, id)
	if e != nil {
		return false
	}
	_, e = r.Store.FailAndRetryOwned(ctx, id, r.owner, err.Error(), result, trace, retryDelay(run.RetryAttempt))
	return e == nil || errors.Is(e, ErrConflict)
}
