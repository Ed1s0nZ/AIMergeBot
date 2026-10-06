package platform

import (
	"fmt"

	"github.com/cloudwego/eino/schema"
)

// The model adapter also uses this for decisions made inside one SDK node.
func primaryDecisionMessages(messages []*schema.Message, trustedPrompt string, current, limit int, navigation ...func() string) []*schema.Message {
	guidance := fmt.Sprintf("\nServer-owned decision budget (navigation only, not source evidence): decision %d of %d.", current, limit)
	if current >= limit {
		guidance += " This is the final primary decision. Return the required strict final JSON now; do not request more tools. Preserve submitted findings and report concrete unresolved investigations and missing evidence in coverage_notes. Never invent evidence or certify safety to finish."
	} else if limit-current <= 2 {
		guidance += " Prepare to finish within the remaining decisions. Update source-linked investigation plans and submit evidenced findings; avoid repeated reads or searching unavailable dependency bodies. Record actual unresolved evidence gaps instead of assuming safety."
	}
	if len(navigation) > 0 && navigation[0] != nil {
		guidance += navigation[0]()
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
