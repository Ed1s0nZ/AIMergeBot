package main

import "pr_agent/internal/platform"

// Completion describes the audit operation, never a certification that code
// is safe. Findings do not select an exit code; incomplete coverage does.
func auditExitCode(run platform.Run, err error) int {
	if run.Status == "incomplete" {
		return 2
	}
	if err != nil || run.Status == "failed" {
		return 1
	}
	if run.Status == "succeeded" || run.Status == "skipped" {
		return 0
	}
	return 1
}
