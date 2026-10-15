package platform

import (
	"context"
	"errors"
	"log"
	"time"
)

// A rejection must not replace an earlier checkpoint with an empty or stale
// local result. The existing worker CAS also makes late cancellation harmless.
func (r *Runner) stopRejectedRepositoryOperation(ctx context.Context, id int64) {
	_ = r.Store.FailWorker(ctx, id, r.owner, "repository authorization interrupted; checkpoint retained")
}

func (r *Runner) finishWithContext(source context.Context, id int64, status, message string, result AuditResult, trace []ToolTrace) bool {
	if r.workerStopped() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if errors.Is(checkRepositoryFence(source), ErrRepositoryUnavailable) {
		r.stopRejectedRepositoryOperation(ctx, id)
		return false
	}
	ctx = copyRepositoryCredentials(ctx, source)
	if err := r.Store.FinishOwned(ctx, id, r.owner, status, message, result, trace); err != nil {
		if errors.Is(err, ErrRepositoryUnavailable) {
			r.stopRejectedRepositoryOperation(ctx, id)
		} else if !errors.Is(err, ErrConflict) {
			log.Printf("audit run %d persistence failed: %v", id, err)
		}
		return false
	}
	return true
}
