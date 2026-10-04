package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xanzy/go-gitlab"
)

// Comment delivery is explicitly recorded before sending. An ambiguous failure is not retried automatically.
func (r *Runner) comment(parent context.Context, id int64) {
	if r.Settings == nil {
		return
	}
	cfg := r.Settings.Snapshot()
	if !cfg.EnableMRComment {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	run, err := r.Store.Run(ctx, id)
	if err != nil || run.Status != "succeeded" {
		return
	}
	repo, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
	if err != nil {
		return
	}
	current, err := repo.Snapshot(ctx, run.ProjectID, run.MRIID)
	if err != nil || current.HeadSHA != run.HeadSHA {
		return
	}
	res, err := r.Store.DB.ExecContext(ctx, `INSERT OR IGNORE INTO platform_comments(run_id,status,updated_at) VALUES(?,'sending',?)`, id, now())
	if err != nil {
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## AIMergeBot 审计 · Run #%d\n\n提交 `%s`\n\n", id, run.HeadSHA))
	b.WriteString(run.Result.Summary)
	for _, f := range run.Result.Findings {
		b.WriteString(fmt.Sprintf("\n\n### %s · %s\n\n`%s:%d` (%s)\n\n%s\n\n触发条件：%s\n\n建议：%s", f.Severity, f.Title, f.Side+":"+f.File, f.Line, f.Confidence, f.Description, f.Trigger, f.Suggestion))
	}
	b.WriteString("\n\n审计结果需人工复核，不构成代码安全保证。")
	_, _, err = repo.Client.Discussions.CreateMergeRequestDiscussion(run.ProjectID, run.MRIID, &gitlab.CreateMergeRequestDiscussionOptions{Body: gitlab.String(b.String())}, gitlab.WithContext(ctx))
	status := "sent"
	if err != nil {
		status = "unknown"
	}
	_, _ = r.Store.DB.ExecContext(ctx, `UPDATE platform_comments SET status=?,updated_at=? WHERE run_id=?`, status, now(), id)
	_ = r.Store.Event(ctx, 0, "comment."+status, fmt.Sprint(id))
}
