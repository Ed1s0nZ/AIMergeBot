package main

import "pr_agent/internal/platform"

func groupedFailureClass(groups []platform.AuditGroupProgress) string {
	result := ""
	for _, group := range groups {
		if group.Status != "failed" {
			continue
		}
		reason := group.StopReason
		if reason == "" {
			reason = "agent_failure"
		}
		if result != "" && result != reason {
			return "multiple_agent_failures"
		}
		result = reason
	}
	return result
}
