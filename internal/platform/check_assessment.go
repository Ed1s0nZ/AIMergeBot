package platform

// CheckAssessment describes this run only. Publishing to a provider is a
// separate operation and must verify the remote current HEAD first.
type CheckAssessment struct {
	RunID            int64  `json:"run_id"`
	HeadSHA          string `json:"head_sha"`
	State            string `json:"state"`
	HighRiskFindings int    `json:"high_risk_findings"`
	UnknownSeverity  int    `json:"unknown_severity"`
	Published        bool   `json:"published"`
	Blocking         bool   `json:"blocking"`
}

func assessRunCheck(r Run) CheckAssessment {
	out := CheckAssessment{RunID: r.ID, HeadSHA: r.HeadSHA, State: "unknown"}
	for _, f := range r.Result.Findings {
		switch f.Severity {
		case "critical", "high":
			out.HighRiskFindings++
		case "medium", "low", "info":
		default:
			out.UnknownSeverity++
		}
	}
	switch r.Status {
	case "pending":
		out.State = "pending"
	case "running":
		out.State = "running"
	case "failed":
		out.State = "failed"
	case "cancelled":
		out.State = "cancelled"
	case "skipped":
		out.State = "skipped"
	case "incomplete":
		out.State = "incomplete"
	case "succeeded":
		switch {
		case r.Error != "":
			out.State = "failed"
		case len(r.Result.CoverageNotes) > 0:
			out.State = "incomplete"
		case !commitID.MatchString(r.HeadSHA) || r.ID <= 0:
			out.State = "unknown"
		case out.UnknownSeverity > 0:
			out.State = "unknown"
		case out.HighRiskFindings > 0:
			out.State = "high_risk"
		default:
			out.State = "completed"
		}
	}
	return out
}
