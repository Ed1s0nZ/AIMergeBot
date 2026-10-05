package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type workerRunState struct {
	owner    string
	ctx      context.Context
	failures chan error
}

func (r *Runner) workerStopped() bool {
	state := r.state.Load()
	return state != nil && state.ctx.Err() != nil
}

// Failures lets the host stop HTTP admission when this worker loses ownership.
func (r *Runner) Failures() <-chan error {
	state := r.state.Load()
	if state == nil {
		return nil
	}
	return state.failures
}

func (r *Runner) leaseLoop(ctx context.Context) {
	ticker := time.NewTicker(workerHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if err := r.maintainLease(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case r.failures <- fmt.Errorf("worker lease maintenance failed: %w", err):
			default:
			}
			r.cancel()
			return
		}
	}
}
func (r *Runner) maintainLease(ctx context.Context) error {
	heartbeat, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	r.mu.Lock()
	ids := make([]int64, 0, len(r.active))
	for id := range r.active {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	if err := r.Store.RenewWorkerInstance(heartbeat, r.owner, ids...); err != nil {
		return err
	}
	live, err := r.Store.LiveWorkerContexts(heartbeat, r.owner, ids)
	if err != nil {
		return err
	}
	r.mu.Lock()
	for _, id := range ids {
		if !live[id] {
			if cancel := r.active[id]; cancel != nil {
				cancel()
			}
		}
	}
	r.mu.Unlock()
	return r.Store.Recover(heartbeat)
}
func (r *Runner) releaseLease() {
	release, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := r.Store.ReleaseWorkerInstance(release, r.owner); err != nil && !errors.Is(err, ErrWorkerLeaseLost) {
		log.Printf("worker lease release failed; expiry will recover interrupted work: %v", err)
	}
}

func (r *Runner) releaseAttemptLease(id int64) {
	r.mu.Lock()
	delete(r.active, id)
	r.mu.Unlock()
	release, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := r.Store.ReleaseRunLease(release, id, r.owner); err != nil {
		log.Printf("audit run %d lease release failed; expiry retains recovery: %v", id, err)
	}
}
