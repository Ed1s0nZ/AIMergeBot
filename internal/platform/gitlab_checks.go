package platform

import (
	"context"
	"fmt"
	"github.com/xanzy/go-gitlab"
	"net/url"
)

type CheckPublication struct {
	State    string `json:"state"`
	Code     string `json:"code,omitempty"`
	RemoteID int    `json:"remote_id,omitempty"`
}

func gitLabCheckState(a CheckAssessment, blocking bool) string {
	switch a.State {
	case "pending":
		return "pending"
	case "running":
		return "running"
	case "completed":
		return "success"
	case "cancelled":
		return "canceled"
	case "skipped":
		return "skipped"
	default:
		if blocking {
			return "failed"
		}
		return "skipped"
	}
}

// Publication is opt-in at the caller. A transport error is never a receipt.
func (g *GitLabRepository) PublishRunCheck(ctx context.Context, r Run, targetURL string, blocking bool, authorize func(context.Context) error) CheckPublication {
	if authorize == nil {
		return CheckPublication{State: "failed", Code: "permission_changed"}
	}
	if err := authorize(ctx); err != nil {
		return checkPreflightFailure(err)
	}
	if r.ID <= 0 || r.ProjectID <= 0 || r.SourceProjectID <= 0 || r.MRIID <= 0 || !commitID.MatchString(r.HeadSHA) || !commitID.MatchString(r.BaseSHA) {
		return CheckPublication{State: "failed", Code: "invalid_identity"}
	}
	if targetURL != "" {
		u, err := url.Parse(targetURL)
		if err != nil || len(targetURL) > 255 || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return CheckPublication{State: "failed", Code: "invalid_target_url"}
		}
	}
	current, _, err := g.Client.MergeRequests.GetMergeRequest(r.ProjectID, r.MRIID, &gitlab.GetMergeRequestsOptions{}, gitlab.WithContext(ctx))
	if err != nil || current == nil {
		return CheckPublication{State: "failed", Code: "preflight_unavailable"}
	}
	source := current.SourceProjectID
	if source <= 0 {
		source = r.ProjectID
	}
	if source != r.SourceProjectID || current.DiffRefs.HeadSha != r.HeadSHA || current.DiffRefs.BaseSha != r.BaseSHA {
		return CheckPublication{State: "stale", Code: "snapshot_changed"}
	}
	if err := authorize(ctx); err != nil {
		return checkPreflightFailure(err)
	}
	a := assessRunCheck(r)
	state := gitLabCheckState(a, blocking)
	name := fmt.Sprintf("aimangebot/run/%d", r.ID)
	mode := "advisory"
	if blocking {
		mode = "blocking"
	}
	description := fmt.Sprintf("AIMergeBot run #%d: %s; high risks=%d; mode=%s", r.ID, a.State, a.HighRiskFindings, mode)
	options := &gitlab.SetCommitStatusOptions{State: gitlab.BuildStateValue(state), Name: &name, Description: &description}
	if targetURL != "" {
		options.TargetURL = &targetURL
	}
	if current.SourceBranch != "" {
		options.Ref = &current.SourceBranch
	}
	receipt, _, err := g.Client.Commits.SetCommitStatus(source, r.HeadSHA, options, gitlab.WithContext(ctx))
	if err != nil {
		return CheckPublication{State: "unknown", Code: "publication_unacknowledged"}
	}
	if receipt == nil || receipt.ID <= 0 || receipt.SHA != r.HeadSHA || receipt.Name != name || receipt.Status != state {
		return CheckPublication{State: "unknown", Code: "receipt_mismatch"}
	}
	return CheckPublication{State: "published", RemoteID: receipt.ID}
}
