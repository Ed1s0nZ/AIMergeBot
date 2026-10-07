package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type botRunStatus struct {
	ID      int64
	HeadSHA string
	Status  string
}

// Authorize the current bound actor and the whole pinned snapshot in the same
// transaction as replay consumption. Return no findings, source or tool trace.
func (s *Store) slackRunStatus(ctx context.Context, integrationID, revision int64, timestamp, signature string, body []byte, at time.Time) (botRunStatus, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return botRunStatus{}, err
	}
	defer tx.Rollback()
	if err := consumeSlackCallbackTx(ctx, tx, integrationID, revision, timestamp, signature, body, at); err != nil {
		return botRunStatus{}, err
	}
	fields, err := url.ParseQuery(string(body))
	if err != nil {
		return botRunStatus{}, ErrConflict
	}
	for _, key := range []string{"user_id", "text"} {
		if len(fields[key]) != 1 {
			return botRunStatus{}, ErrConflict
		}
	}
	external := fields.Get("user_id")
	text := fields.Get("text")
	if !slackUserID.MatchString(external) || !strings.HasPrefix(text, "status ") {
		return botRunStatus{}, ErrConflict
	}
	runID, err := strconv.ParseInt(strings.TrimPrefix(text, "status "), 10, 64)
	if err != nil || runID <= 0 {
		return botRunStatus{}, ErrConflict
	}
	var actor int64
	if err := tx.QueryRowContext(ctx, `SELECT b.user_id FROM platform_bot_bindings b JOIN platform_users u ON u.id=b.user_id AND u.disabled=0 WHERE b.integration_id=? AND b.workspace_id=? AND b.external_user_id=?`, integrationID, fields.Get("team_id"), external).Scan(&actor); err != nil {
		return botRunStatus{}, err
	}
	var snap Snapshot
	var policy, status string
	if err := tx.QueryRowContext(ctx, `SELECT project_id,source_project_id,mr_iid,base_sha,head_sha,status,CASE WHEN length(CAST(audit_policy_json AS BLOB))<=65536 THEN audit_policy_json ELSE '' END FROM platform_runs WHERE id=?`, runID).Scan(&snap.ProjectID, &snap.SourceProjectID, &snap.MRIID, &snap.BaseSHA, &snap.HeadSHA, &status, &policy); err != nil {
		return botRunStatus{}, err
	}
	if err := json.Unmarshal([]byte(policy), &snap.AuditPolicy); err != nil {
		return botRunStatus{}, ErrConflict
	}
	if _, err := requireSnapshotRole(ctx, tx, snap, actor, "viewer"); err != nil {
		return botRunStatus{}, err
	}
	if err := requireTicketProjectsEnabled(ctx, tx, snap); err != nil {
		return botRunStatus{}, err
	}
	var scoped bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_integrations i,json_each(CASE WHEN json_valid(i.project_ids) THEN i.project_ids ELSE '[]' END) scope WHERE i.id=? AND scope.value=?)`, integrationID, snap.ProjectID).Scan(&scoped); err != nil {
		return botRunStatus{}, err
	}
	if !scoped || !commitID.MatchString(snap.HeadSHA) {
		return botRunStatus{}, ErrConflict
	}
	switch status {
	case "pending", "running", "succeeded", "failed", "incomplete", "cancelled", "skipped":
	default:
		return botRunStatus{}, ErrConflict
	}
	out := botRunStatus{ID: runID, HeadSHA: snap.HeadSHA, Status: status}
	return out, tx.Commit()
}
