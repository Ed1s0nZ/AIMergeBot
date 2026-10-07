package platform

type CheckRules struct {
	Enabled           bool   `json:"enabled"`
	Mode              string `json:"mode"`
	MinimumSeverity   string `json:"minimum_severity"`
	BlockOnFailure    bool   `json:"block_on_failure"`
	BlockOnIncomplete bool   `json:"block_on_incomplete"`
	Publisher         int64  `json:"publisher"`
}

func normalizeCheckRules(p *CheckRules) error {
	if p == nil {
		return nil
	}
	if p.Mode == "" {
		p.Mode = "advisory"
	}
	if p.MinimumSeverity == "" {
		p.MinimumSeverity = "high"
	}
	if (p.Mode != "advisory" && p.Mode != "blocking") || notificationSeverity(p.MinimumSeverity) == 0 {
		return ErrWorkflowPolicy
	}
	return nil
}

func effectiveCheckRules(r Run) CheckRules {
	p := CheckRules{Mode: "advisory", MinimumSeverity: "high", BlockOnFailure: true, BlockOnIncomplete: true}
	if r.AuditPolicy != nil && r.AuditPolicy.Workflow != nil && r.AuditPolicy.Workflow.Checks != nil {
		p = *r.AuditPolicy.Workflow.Checks
	}
	_ = normalizeCheckRules(&p)
	return p
}

func checkStateForRules(r Run, p CheckRules) (string, int) {
	if normalizeCheckRules(&p) != nil {
		return "skipped", 0
	}
	a := assessRunCheck(r)
	count := 0
	for _, f := range r.Result.Findings {
		if notificationSeverity(f.Severity) >= notificationSeverity(p.MinimumSeverity) {
			count++
		}
	}
	switch a.State {
	case "pending":
		return "pending", count
	case "running":
		return "running", count
	case "cancelled":
		return "canceled", count
	case "skipped":
		return "skipped", count
	case "failed":
		if p.Mode == "blocking" && p.BlockOnFailure {
			return "failed", count
		}
		return "skipped", count
	case "incomplete":
		if p.Mode == "blocking" && p.BlockOnIncomplete {
			return "failed", count
		}
		return "skipped", count
	case "unknown":
		if p.Mode == "blocking" {
			return "failed", count
		}
		return "skipped", count
	default:
		if count > 0 {
			if p.Mode == "blocking" {
				return "failed", count
			}
			return "skipped", count
		}
		return "success", count
	}
}
