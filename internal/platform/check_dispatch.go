package platform

import (
	"context"
	"fmt"
	"strings"
)

// DispatchRunCheck consumes only an explicitly authorized durable queue row.
func (r *Runner) DispatchRunCheck(ctx context.Context) (bool, error) {
	d, err := r.Store.ClaimRunCheck(ctx)
	if err != nil {
		return false, err
	}
	finish := func(result CheckPublication) (bool, error) { return true, r.Store.FinishRunCheck(ctx, d, result) }
	run, err := r.Store.CheckPublicationRun(ctx, d)
	if err != nil {
		return finish(checkPreflightFailure(err))
	}
	if r.Settings == nil || run.AuditPolicy == nil {
		return finish(CheckPublication{State: "failed", Code: "preflight_unavailable"})
	}
	cfg := r.Settings.Snapshot()
	if strings.TrimRight(cfg.GitLab.URL, "/") != run.AuditPolicy.RepositoryURL {
		return finish(CheckPublication{State: "failed", Code: "preflight_unavailable"})
	}
	repo, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
	if err != nil {
		return finish(CheckPublication{State: "failed", Code: "preflight_unavailable"})
	}
	target := ""
	if cfg.PublicURL != "" {
		target = strings.TrimRight(cfg.PublicURL, "/") + fmt.Sprintf("/#/runs/%d", run.ID)
	}
	var namespace string
	if err = r.Store.DB.QueryRowContext(ctx, `SELECT namespace FROM platform_comment_installation WHERE id=1`).Scan(&namespace); err != nil {
		return finish(CheckPublication{State: "failed", Code: "preflight_unavailable"})
	}
	authorize := func(ctx context.Context) error { _, err := r.Store.CheckPublicationRun(ctx, d); return err }
	return finish(repo.PublishRunCheck(ctx, run, target, d.Blocking, namespace, authorize))
}
