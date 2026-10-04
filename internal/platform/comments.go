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
	if run.AuditPolicy != nil && strings.TrimRight(cfg.GitLab.URL, "/") != run.AuditPolicy.RepositoryURL {
		return
	}
	repo, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
	if err != nil {
		return
	}
	current, err := repo.Snapshot(ctx, run.ProjectID, run.MRIID)
	if err != nil || current.HeadSHA != run.HeadSHA || current.BaseSHA != run.BaseSHA {
		return
	}
	claimed, err := r.Store.claimOwnedComment(ctx, id, r.owner)
	if err != nil || !claimed {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## AIMergeBot 审计 · Run #%d\n\n提交 `%s`\n\n", id, run.HeadSHA))
	b.WriteString(run.Result.Summary)
	for _, f := range run.Result.Findings {
		b.WriteString(fmt.Sprintf("\n\n### %s · %s\n\n`%s:%d` (%s)\n\n%s\n\n触发条件：%s\n\n建议：%s", f.Severity, f.Title, f.Side+":"+f.File, f.Line, f.Confidence, f.Description, f.Trigger, f.Suggestion))
	}
	b.WriteString("\n\n审计结果需人工复核，不构成代码安全保证。")
	if !r.Store.commentOwnerValid(ctx, id, r.owner) {
		return
	}
	_, _, err = repo.Client.Discussions.CreateMergeRequestDiscussion(run.ProjectID, run.MRIID, &gitlab.CreateMergeRequestDiscussionOptions{Body: gitlab.String(b.String())}, gitlab.WithContext(ctx))
	status := "sent"
	if err != nil {
		status = "unknown"
	}
	if saved, e := r.Store.finishOwnedComment(ctx, id, r.owner, status); e == nil && saved {
		_ = r.Store.Event(ctx, 0, "comment."+status, fmt.Sprint(id))
	}
}
