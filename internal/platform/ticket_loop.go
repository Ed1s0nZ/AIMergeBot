package platform

import (
	"context"
	"time"
)

// Only explicit, authorized reservations enter this queue. Unknown outcomes
// remain terminal so a restart cannot repeat an unacknowledged remote write.
func (r *Runner) ticketLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 5 && ctx.Err() == nil; i++ {
				publicURL := ""
				if r.Settings != nil {
					publicURL = r.Settings.Snapshot().PublicURL
				}
				attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
				claimed, _ := r.Store.DispatchFindingTicket(attempt, publicURL, r.ticketClient)
				cancel()
				if !claimed {
					break
				}
			}
		}
	}
}
