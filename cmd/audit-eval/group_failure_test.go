package main

import (
	"pr_agent/internal/platform"
	"testing"
)

func TestGroupedFailureClass(t *testing.T) {
	groups := []platform.AuditGroupProgress{{Status: "completed", StopReason: "ignored"}, {Status: "failed", StopReason: "agent_step_budget"}}
	if groupedFailureClass(groups) != "agent_step_budget" {
		t.Fatal("failed cause lost")
	}
	groups = append(groups, platform.AuditGroupProgress{Status: "failed", StopReason: "deadline"})
	if groupedFailureClass(groups) != "multiple_agent_failures" {
		t.Fatal("mixed causes lost")
	}
	if groupedFailureClass([]platform.AuditGroupProgress{{Status: "failed"}}) != "agent_failure" {
		t.Fatal("missing historical cause")
	}
}
