package platform

import (
	"context"
	"time"
)

// The DB transition is authoritative; interrupt requests best-effort to release
// the local worker promptly, while checkpoints/finish CAS reject late writes.
func (r *Runner) interruptRevoked(project int, user int64) {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	rows, err := r.Store.DB.QueryContext(ctx, `SELECT id FROM platform_runs WHERE (project_id=? OR source_project_id=? OR EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id AND c.project_id=?)) AND requested_by=? AND status='cancelled' AND error='project execution permission revoked'`, project, project, project, user)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			return
		}
		r.mu.Lock()
		if stop := r.active[id]; stop != nil {
			stop()
		}
		r.mu.Unlock()
	}
}

func (r *Runner) interruptContextRevoked(project int) {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	rows, err := r.Store.DB.QueryContext(ctx, `SELECT id FROM platform_runs WHERE (project_id=? OR EXISTS(SELECT 1 FROM platform_run_context_repositories c WHERE c.run_id=platform_runs.id AND c.project_id=?)) AND status='cancelled' AND error='context repository authorization changed'`, project, project)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			return
		}
		r.mu.Lock()
		if cancel := r.active[id]; cancel != nil {
			cancel()
		}
		r.mu.Unlock()
	}
}
