package platform

import (
	"context"
	"time"
)

func (r *Runner) checkLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkCycle(ctx)
		}
	}
}

func (r *Runner) checkCycle(ctx context.Context) {
	collect, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, _ = r.Store.CollectRunChecks(collect)
	cancel()
	// A rejected candidate must not prevent other authorized queued results
	// from being dispatched. Bound each cycle to keep cancellation responsive.
	for i := 0; i < 5 && ctx.Err() == nil; i++ {
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		claimed, _ := r.DispatchRunCheck(attempt)
		cancel()
		if !claimed {
			return
		}
	}
}
