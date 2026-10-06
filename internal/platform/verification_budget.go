package platform

import (
	"context"
	"time"
)

// Supplemental reviewers share one pool. Consumers run serially; source-tool
// callbacks must not allocate or replenish it.
type verificationBudget struct {
	ctx       context.Context
	cancel    context.CancelFunc
	remaining int
}

func newVerificationBudget(ctx context.Context) *verificationBudget {
	duration := 60 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		left := time.Until(deadline) - 10*time.Second
		if left < duration {
			duration = left
		}
	}
	if duration < 0 {
		duration = 0
	}
	phase, cancel := context.WithTimeout(ctx, duration)
	return &verificationBudget{ctx: phase, cancel: cancel, remaining: 40}
}

func (b *verificationBudget) nextLimit() int {
	if b.ctx.Err() != nil || b.remaining <= 0 {
		return 0
	}
	if b.remaining < 10 {
		return b.remaining
	}
	return 10
}

func (b *verificationBudget) consume(calls, limit int) {
	if calls > limit {
		calls = limit
	}
	b.remaining -= calls
}
