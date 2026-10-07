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
	_, retryable := retryFailure(err)
	return retryable
}

func retryFailure(err error) (*RetryInfo, bool) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, false
	}
	var upstream *upstreamError
	if errors.As(err, &upstream) {
		info := upstream.info
		return &info, info.Kind == "temporary_network" || info.HTTPStatus == 429 || (info.HTTPStatus >= 500 && info.HTTPStatus <= 599)
	}
	info := &RetryInfo{Source: "unknown"}
	var api *modelopenai.APIError
	var request *modelopenai.RequestError
	var git *gitlab.ErrorResponse
	var network net.Error
	if errors.As(err, &api) {
		info.HTTPStatus = api.HTTPStatusCode
		info.Source = "model"
	} else if errors.As(err, &request) {
		info.HTTPStatus = request.HTTPStatusCode
		info.Source = "model"
	} else if errors.As(err, &git) && git.Response != nil {
		info.HTTPStatus = git.Response.StatusCode
		info.Source = "gitlab"
		info.RetryAfterUntil, info.HeaderState = parseRetryAfter(git.Response.Header.Get("Retry-After"), time.Now())
	}
	if info.HTTPStatus != 0 {
		if info.HeaderState == "" {
			info.HeaderState = "absent"
		}
		if info.HTTPStatus == 429 {
			info.Kind = "rate_limit"
			return info, true
		}
		if info.HTTPStatus >= 500 && info.HTTPStatus <= 599 {
			info.Kind = "upstream_server"
			return info, true
		}
		return nil, false
	}
	if errors.As(err, &network) && (network.Timeout() || network.Temporary()) {
		info.Kind = "temporary_network"
		return info, true
	}
	return nil, false
}

// FailAndRetry atomically records the failed attempt and creates its delayed child.
func (s *Store) FailAndRetry(ctx context.Context, id int64, message string, result AuditResult, trace []ToolTrace, delay time.Duration) (int64, error) {
	return s.FailAndRetryOwned(ctx, id, "", message, result, trace, delay)
}
func (s *Store) FailAndRetryOwned(ctx context.Context, id int64, owner, message string, result AuditResult, trace []ToolTrace, delay time.Duration) (int64, error) {
	return s.failAndRetry(ctx, id, owner, message, result, trace, delay, false, nil)
}
func (s *Store) recoverAttempt(ctx context.Context, id int64) (int64, error) {
	return s.failAndRetry(ctx, id, "", "worker lease expired or service interrupted; checkpoint retained", AuditResult{}, nil, 0, true, &RetryInfo{Kind: "worker_interrupted", Source: "worker"})
}
func (s *Store) failAndRetry(ctx context.Context, id int64, owner, message string, result AuditResult, trace []ToolTrace, delay time.Duration, recovery bool, cause *RetryInfo) (int64, error) {
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
	var attempt int
	var policy string
	predicate := workerFenceSQL
	args := []any{id, owner}
	if recovery {
		predicate = expiredWorkerSQL
		args = []any{id}
	}
	if err = tx.QueryRowContext(ctx, `SELECT retry_attempt,policy_version FROM platform_runs WHERE id=? AND status='running'`+predicate, args...).Scan(&attempt, &policy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
		return 0, err
	}
	if recovery {
		delay = retryDelay(attempt)
	}
	info := RetryInfo{Kind: "transient_unknown", Source: "unknown"}
	if cause != nil {
		info = *cause
	}
	if until, e := time.Parse(time.RFC3339Nano, info.RetryAfterUntil); e == nil {
		if remaining := time.Until(until); remaining > delay {
			delay = remaining
		}
	}
	info.State = "scheduled"
	if attempt >= maxAutomaticRetries {
		info.State = "exhausted"
	} else if policy != PolicyVersion {
		info.State = "policy_changed"
	} else if info.HeaderState == "exceeds_limit" {
		info.State = "wait_exceeds_limit"
	}
	if state, e := retryBudgetStoppingState(ctx, tx, id, trace, recovery); e != nil {
		return 0, e
	} else if state != "" {
		info.State = state
	}
	if info.State == "scheduled" {
		if err := requireLegacyRunRepository(ctx, tx, id); err != nil {
			if !errors.Is(err, ErrRepositoryUnavailable) {
				return 0, err
			}
			info.State = "repository_unavailable"
		}
	}
	retryAt := ""
	if info.State == "scheduled" {
		info.DelaySeconds = int64((delay + time.Second - 1) / time.Second)
		info.EligibleAt = time.Now().UTC().Add(delay).Format(time.RFC3339Nano)
		if delay > 0 {
			retryAt = info.EligibleAt
		}
	}
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return 0, err
	}
	var res sql.Result
	if recovery {
		res, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error=?,retry_info_json=?,finished_at=? WHERE id=? AND status='running'`+expiredWorkerSQL, message, string(infoJSON), now(), id)
	} else {
		res, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='failed',error=?,result_json=?,trace_json=?,retry_info_json=?,finished_at=? WHERE id=? AND status='running'`+workerFenceSQL, message, string(data), string(tr), string(infoJSON), now(), id, owner)
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
	if info.State != "scheduled" {
		return 0, tx.Commit()
	}
	res, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO platform_runs(project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,status,created_at,requested_by,policy_version,policy_digest,audit_policy_json,retry_parent_id,retry_attempt,retry_at,retry_info_json) SELECT project_id,mr_iid,source_project_id,diff_version_id,base_sha,head_sha,title,url,'pending',?,requested_by,policy_version,policy_digest,audit_policy_json,id,retry_attempt+1,?,retry_info_json FROM platform_runs WHERE id=? AND retry_attempt<? AND policy_version=?`, now(), retryAt, id, maxAutomaticRetries, PolicyVersion)
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
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_run_context_repositories(run_id,project_id,sha) SELECT ?,project_id,sha FROM platform_run_context_repositories WHERE run_id=?`, child, id); err != nil {
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
	cause, retryable := retryFailure(err)
	if !retryable {
		return false
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	run, e := r.Store.Run(ctx, id)
	if e != nil {
		return false
	}
	message := err.Error()
	var upstream *upstreamError
	if errors.As(err, &upstream) {
		message = upstream.Error()
	}
	_, e = r.Store.failAndRetry(ctx, id, r.owner, message, result, trace, retryDelay(run.RetryAttempt), false, cause)
	return e == nil || errors.Is(e, ErrConflict)
}
