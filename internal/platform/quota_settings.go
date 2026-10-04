package platform

import (
	"errors"
	"fmt"
)

var ErrAuditQuotas = errors.New("invalid audit quotas")

// AuditQuotas limits queue size and audit starts, not provider tokens or money.
type AuditQuotas struct {
	OutstandingGlobal  int `yaml:"outstanding_global" json:"outstanding_global"`
	OutstandingProject int `yaml:"outstanding_project" json:"outstanding_project"`
	OutstandingUser    int `yaml:"outstanding_user" json:"outstanding_user"`
	RunningProject     int `yaml:"running_project" json:"running_project"`
	RunningUser        int `yaml:"running_user" json:"running_user"`
	DailyGlobal        int `yaml:"daily_global" json:"daily_global"`
	DailyProject       int `yaml:"daily_project" json:"daily_project"`
	DailyUser          int `yaml:"daily_user" json:"daily_user"`
}

func defaultAuditQuotas(q *AuditQuotas) {
	defaults := []struct {
		value    *int
		fallback int
	}{
		{&q.OutstandingGlobal, 200}, {&q.OutstandingProject, 50}, {&q.OutstandingUser, 20},
		{&q.RunningProject, 2}, {&q.RunningUser, 2}, {&q.DailyGlobal, 1000}, {&q.DailyProject, 200}, {&q.DailyUser, 100},
	}
	for _, entry := range defaults {
		if *entry.value == 0 {
			*entry.value = entry.fallback
		}
	}
}
func validateAuditQuotas(q AuditQuotas) error {
	for _, entry := range []struct {
		name       string
		value, max int
	}{
		{"outstanding_global", q.OutstandingGlobal, 10000}, {"outstanding_project", q.OutstandingProject, 10000}, {"outstanding_user", q.OutstandingUser, 10000},
		{"running_project", q.RunningProject, 16}, {"running_user", q.RunningUser, 16},
		{"daily_global", q.DailyGlobal, 100000}, {"daily_project", q.DailyProject, 100000}, {"daily_user", q.DailyUser, 100000},
	} {
		if entry.value < 1 || entry.value > entry.max {
			return fmt.Errorf("%w: %s must be 1–%d", ErrAuditQuotas, entry.name, entry.max)
		}
	}
	if q.RunningProject > q.OutstandingProject || q.RunningUser > q.OutstandingUser {
		return fmt.Errorf("%w: running limits cannot exceed corresponding outstanding limits", ErrAuditQuotas)
	}
	return nil
}
