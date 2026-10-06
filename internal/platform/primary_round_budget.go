package platform

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// primaryRoundRewriter adds bounded navigation, never source evidence. Each
// auditor owns its counter; the SDK remains responsible for the hard limit.
func primaryRoundRewriter(limit int, trustedPrompt string, rewrite func(context.Context, []*schema.Message) []*schema.Message) func(context.Context, []*schema.Message) []*schema.Message {
	var mu sync.Mutex
	round := 0
	return func(ctx context.Context, messages []*schema.Message) []*schema.Message {
		messages = rewrite(ctx, messages)
		mu.Lock()
		round++
		current := round
		mu.Unlock()
		guidance := fmt.Sprintf("\nServer-owned decision budget (navigation only, not source evidence): decision %d of %d.", current, limit)
		if current >= limit {
			guidance += " This is the final primary decision. Return the required strict final JSON now; do not request more tools. Preserve submitted findings and report concrete unresolved investigations and missing evidence in coverage_notes. Never invent evidence or certify safety to finish."
		} else if limit-current <= 2 {
			guidance += " Prepare to finish within the remaining decisions. Update source-linked investigation plans and submit evidenced findings; avoid repeated reads or searching unavailable dependency bodies. Record actual unresolved evidence gaps instead of assuming safety."
		}
		out := append([]*schema.Message(nil), messages...)
		system := schema.SystemMessage(trustedPrompt + guidance)
		if len(out) > 0 && out[0] != nil && out[0].Role == schema.System {
			copy := *out[0]
			copy.Content = system.Content
			out[0] = &copy
		} else {
			out = append([]*schema.Message{system}, out...)
		}
		return out
	}
}
