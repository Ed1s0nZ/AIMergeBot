package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Reaudit a named immutable snapshot, never silently switch to the latest HEAD.
// Replay receipt, current authorization, quotas and admission share one commit.
func (s *Store) slackReaudit(ctx context.Context, integrationID, revision int64, timestamp, signature string, body []byte, at time.Time) (int64, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	if err := consumeSlackCallbackTx(ctx, tx, integrationID, revision, timestamp, signature, body, at); err != nil {
		return 0, false, err
	}
	fields, err := url.ParseQuery(string(body))
	if err != nil || len(fields["user_id"]) != 1 || len(fields["text"]) != 1 || !slackUserID.MatchString(fields.Get("user_id")) {
		return 0, false, ErrConflict
	}
	args := strings.Split(fields.Get("text"), " ")
	if len(args) != 3 || args[0] != "reaudit" || !commitID.MatchString(args[2]) {
		return 0, false, ErrConflict
	}
	parent, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || parent <= 0 {
		return 0, false, ErrConflict
	}
	var actor int64
	if err := tx.QueryRowContext(ctx, `SELECT b.user_id FROM platform_bot_bindings b JOIN platform_users u ON u.id=b.user_id AND u.disabled=0 WHERE b.integration_id=? AND b.workspace_id=? AND b.external_user_id=?`, integrationID, fields.Get("team_id"), fields.Get("user_id")).Scan(&actor); err != nil {
		return 0, false, err
	}
	var snap Snapshot
	var policy, status, version string
	if err := tx.QueryRowContext(ctx, `SELECT project_id,source_project_id,mr_iid,diff_version_id,base_sha,head_sha,title,url,status,policy_version,CASE WHEN length(CAST(audit_policy_json AS BLOB))<=65536 THEN audit_policy_json ELSE '' END FROM platform_runs WHERE id=?`, parent).Scan(&snap.ProjectID, &snap.SourceProjectID, &snap.MRIID, &snap.DiffVersionID, &snap.BaseSHA, &snap.HeadSHA, &snap.Title, &snap.URL, &status, &version, &policy); err != nil {
		return 0, false, err
	}
	if version != PolicyVersion || snap.HeadSHA != args[2] || !commitID.MatchString(snap.BaseSHA) || json.Unmarshal([]byte(policy), &snap.AuditPolicy) != nil {
		return 0, false, ErrConflict
	}
	switch status {
	case "succeeded", "failed", "incomplete", "cancelled", "skipped":
	default:
		return 0, false, ErrConflict
	}
	if _, err := requireSnapshotRole(ctx, tx, snap, actor, "operator"); err != nil {
		return 0, false, err
	}
	if err := requireTicketProjectsEnabled(ctx, tx, snap); err != nil {
		return 0, false, err
	}
	var scoped bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_integrations i,json_each(CASE WHEN json_valid(i.project_ids) THEN i.project_ids ELSE '[]' END) scope WHERE i.id=? AND scope.value=?)`, integrationID, snap.ProjectID).Scan(&scoped); err != nil {
		return 0, false, err
	}
	if !scoped {
		return 0, false, ErrConflict
	}
	id, created, err := s.enqueueTx(ctx, tx, snap, actor, true, true)
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'bot.reaudit.requested',?,?)`, actor, id, now()); err != nil {
		return 0, false, err
	}
	return id, created, tx.Commit()
}
