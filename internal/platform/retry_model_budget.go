package platform

import (
	"context"
	"encoding/json"
	"errors"
)

type retryBudgetRow struct {
	id, parent                                           int64
	attempt                                              int
	project, source, mr, version                         int
	base, head, policyVersion, digest, status, errorText string
	policy                                               *AuditPolicy
	trace                                                []ToolTrace
}

func loadRetryBudgetRow(ctx context.Context, q queryRower, id int64) (retryBudgetRow, error) {
	row := retryBudgetRow{id: id}
	var policy, trace string
	err := q.QueryRowContext(ctx, `SELECT retry_parent_id,retry_attempt,project_id,source_project_id,mr_iid,diff_version_id,base_sha,head_sha,policy_version,policy_digest,status,error,audit_policy_json,trace_json FROM platform_runs WHERE id=?`, id).Scan(&row.parent, &row.attempt, &row.project, &row.source, &row.mr, &row.version, &row.base, &row.head, &row.policyVersion, &row.digest, &row.status, &row.errorText, &policy, &trace)
	if err != nil {
		return row, err
	}
	if err = json.Unmarshal([]byte(policy), &row.policy); err != nil {
		return row, err
	}
	err = json.Unmarshal([]byte(trace), &row.trace)
	return row, err
}
func sameRetryBudgetIdentity(a, b retryBudgetRow) bool {
	return a.project == b.project && a.source == b.source && a.mr == b.mr && a.version == b.version && a.base == b.base && a.head == b.head && a.policyVersion == b.policyVersion && a.digest == b.digest && policyDigest(a.policy) == policyDigest(b.policy)
}
func retryBudgetAncestors(ctx context.Context, q queryRower, id int64) ([]retryBudgetRow, error) {
	rows := []retryBudgetRow{}
	seen := map[int64]bool{}
	for id > 0 {
		if seen[id] || len(rows) > maxAutomaticRetries {
			return nil, ErrConflict
		}
		seen[id] = true
		row, err := loadRetryBudgetRow(ctx, q, id)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			child := rows[len(rows)-1]
			if !sameRetryBudgetIdentity(child, row) || child.attempt != row.attempt+1 {
				return nil, ErrConflict
			}
		}
		rows = append(rows, row)
		id = row.parent
	}
	if len(rows) == 0 || rows[len(rows)-1].attempt != 0 {
		return nil, ErrConflict
	}
	return rows, nil
}

// Clamp arithmetic to the stopping threshold. Unknown request markers/errors
// are not free; they prevent an automatic request from receiving fresh budget.
func retryReportedTokens(rows []retryBudgetRow, limit int) (int, bool) {
	used := 0
	unknown := false
	for _, row := range rows {
		for _, tr := range row.trace {
			if tr.Name != "model" {
				continue
			}
			if !tr.UsageReported || tr.Error != "" || tr.PromptTokens < 0 || tr.CompletionTokens < 0 || tr.TotalTokens < 0 {
				unknown = true
				continue
			}
			n := int64(tr.PromptTokens) + int64(tr.CompletionTokens)
			if int64(tr.TotalTokens) > n {
				n = int64(tr.TotalTokens)
			}
			if n <= 0 {
				unknown = true
				continue
			}
			if n >= int64(limit-used) {
				used = limit
			} else {
				used += int(n)
			}
		}
	}
	return used, unknown
}
func retryBudgetStoppingState(ctx context.Context, q queryRower, id int64, current []ToolTrace, recovery bool) (string, error) {
	currentRow, err := loadRetryBudgetRow(ctx, q, id)
	if err != nil {
		return "", err
	}
	if currentRow.policy == nil || currentRow.policy.ModelBudget.MaxTokens <= 0 {
		return "", nil
	}
	rows, err := retryBudgetAncestors(ctx, q, id)
	if err != nil {
		return "", err
	}
	if !recovery {
		rows[0].trace = current
	}
	used, unknown := retryReportedTokens(rows, rows[0].policy.ModelBudget.MaxTokens)
	if unknown {
		return "model_usage_unknown", nil
	}
	if used >= rows[0].policy.ModelBudget.MaxTokens {
		return "model_budget_exhausted", nil
	}
	return "", nil
}
func (s *Store) seedRetryModelBudget(ctx context.Context, run Run) (context.Context, error) {
	if run.AuditPolicy == nil || run.AuditPolicy.ModelBudget.MaxTokens <= 0 {
		return ctx, nil
	}
	rows, err := retryBudgetAncestors(ctx, s.DB, run.ID)
	if err != nil {
		return ctx, err
	}
	// The running attempt starts empty; its ancestors retain their own receipts.
	rows[0].trace = nil
	limit := run.AuditPolicy.ModelBudget.MaxTokens
	used, unknown := retryReportedTokens(rows, limit)
	if unknown {
		return ctx, ErrModelUsageUnknown
	}
	if used >= limit {
		return ctx, ErrModelTokenBudget
	}
	return context.WithValue(ctx, modelBudgetKey{}, &modelTokenBudget{limit: limit, used: used}), nil
}

type RetryAttemptUsage struct {
	RunID  int64      `json:"run_id"`
	Status string     `json:"status"`
	Usage  ModelUsage `json:"usage"`
}
type RetryChainUsage struct {
	RootID   int64               `json:"root_id"`
	Attempts []RetryAttemptUsage `json:"attempts"`
	Usage    ModelUsage          `json:"usage"`
}

func (s *Store) RetryChainUsage(ctx context.Context, id int64) (RetryChainUsage, error) {
	ancestors, err := retryBudgetAncestors(ctx, s.DB, id)
	if err != nil {
		return RetryChainUsage{}, err
	}
	root := ancestors[len(ancestors)-1]
	out := RetryChainUsage{RootID: root.id, Attempts: []RetryAttemptUsage{}}
	rows := []retryBudgetRow{}
	current := root
	for {
		if len(rows) > maxAutomaticRetries {
			return out, ErrConflict
		}
		rows = append(rows, current)
		var count int
		var next int64
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(id),0) FROM platform_runs WHERE retry_parent_id=?`, current.id).Scan(&count, &next); err != nil {
			return out, err
		}
		if count == 0 {
			break
		}
		if count != 1 {
			return out, ErrConflict
		}
		child, e := loadRetryBudgetRow(ctx, s.DB, next)
		if e != nil {
			return out, e
		}
		if !sameRetryBudgetIdentity(current, child) || child.attempt != current.attempt+1 {
			return out, ErrConflict
		}
		current = child
	}
	settings := ModelBudgetSettings{}
	different := false
	if root.policy != nil {
		settings = root.policy.ModelBudget
		different = root.policy.VerificationModel != "" && root.policy.VerificationModel != root.policy.Model
	}
	trace := []ToolTrace{}
	complete := true
	for _, row := range rows {
		terminal := (row.status == "succeeded" || row.status == "incomplete") && row.errorText == ""
		complete = complete && terminal
		trace = append(trace, row.trace...)
		out.Attempts = append(out.Attempts, RetryAttemptUsage{RunID: row.id, Status: row.status, Usage: SummarizeModelUsage(row.trace, settings, terminal, different)})
	}
	out.Usage = SummarizeModelUsage(trace, settings, complete, different)
	return out, nil
}
func budgetInterruptionResult(err error) AuditResult {
	reason := "Unable to verify inherited retry model budget"
	if errors.Is(err, ErrModelUsageUnknown) {
		reason = "Earlier retry model request has unknown usage; automatic requests stopped"
	}
	if errors.Is(err, ErrModelTokenBudget) {
		reason = "Retry chain model token stopping threshold reached"
	}
	return AuditResult{Findings: []Finding{}, Summary: "Automatic retry stopped before model request", CoverageNotes: []string{reason}}
}
