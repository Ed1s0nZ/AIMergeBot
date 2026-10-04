package platform

import (
	"context"
	"log"
	"time"

	"github.com/xanzy/go-gitlab"
)

// Poll uses persisted seen SHAs as the non-retroactive baseline; it never writes fake completed runs.
func (r *Runner) Poll(ctx context.Context) {
	initialized := map[int]bool{}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if r.Settings != nil {
			cfg := r.Settings.Snapshot()
			if cfg.EnablePolling {
				projects, err := r.Store.Projects(ctx)
				if err != nil {
					log.Printf("poll project query failed: %v", err)
				} else {
					repo, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
					if err != nil {
						log.Printf("poll client unavailable")
					} else {
						for _, p := range projects {
							if !p.Enabled {
								continue
							}
							complete := true
							for page := 1; page <= 10000; page++ {
								mrs, res, err := repo.Client.MergeRequests.ListProjectMergeRequests(p.ID, &gitlab.ListProjectMergeRequestsOptions{State: gitlab.String("opened"), ListOptions: gitlab.ListOptions{Page: page, PerPage: 100}}, gitlab.WithContext(ctx))
								if err != nil {
									log.Printf("poll project %d failed", p.ID)
									complete = false
									break
								}
								for _, mr := range mrs {
									snap, err := repo.Snapshot(ctx, p.ID, mr.IID)
									if err != nil {
										complete = false
										continue
									}
									var previous string
									_ = r.Store.DB.QueryRowContext(ctx, `SELECT head_sha FROM platform_poll_seen WHERE project_id=? AND mr_iid=?`, p.ID, mr.IID).Scan(&previous)
									baseline := !initialized[p.ID] && !cfg.ScanExistingMRs && previous == ""
									if !baseline && previous != snap.HeadSHA {
										snap.AuditPolicy = capturePolicy(cfg)
										if _, _, err = r.Store.Enqueue(ctx, snap, 0, false); err != nil {
											complete = false
											continue
										}
									}
									if _, err = r.Store.DB.ExecContext(ctx, `INSERT INTO platform_poll_seen(project_id,mr_iid,head_sha) VALUES(?,?,?) ON CONFLICT(project_id,mr_iid) DO UPDATE SET head_sha=excluded.head_sha`, p.ID, mr.IID, snap.HeadSHA); err != nil {
										complete = false
									}
								}
								if res.NextPage == 0 {
									break
								}
							}
							if complete {
								initialized[p.ID] = true
							}
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
