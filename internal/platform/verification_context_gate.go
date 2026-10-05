package platform

import (
	"fmt"
	"strings"
)

// Configuration identifies related sources. Missing inspection is uncertainty,
// never negative evidence and never permission to upgrade a conditional claim.
func gateContextVerification(v *FindingVerification, snap Snapshot, inspected map[int]bool) {
	if v.Status != "supported" {
		return
	}
	missing := []string{}
	for _, source := range contextPolicyItems(snap) {
		if !inspected[source.ProjectID] {
			missing = append(missing, fmt.Sprint(source.ProjectID))
		}
	}
	if len(missing) == 0 {
		return
	}
	note := "Independent verification did not cite fresh source evidence from configured context repositories: " + strings.Join(missing, ", ")
	v.Status = "inconclusive"
	v.Limitations = append(v.Limitations, note)
	reason := []rune(note + ". Model support proposal was not accepted. Unconfirmed model explanation: " + v.Reason)
	if len(reason) > 1000 {
		reason = reason[:999]
		reason = append(reason, '…')
	}
	v.Reason = string(reason)
}
