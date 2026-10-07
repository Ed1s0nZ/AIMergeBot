package platform

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/xanzy/go-gitlab"
	"net/url"
	"strings"
)

type CheckPublication struct {
	State    string `json:"state"`
	Code     string `json:"code,omitempty"`
	RemoteID int    `json:"remote_id,omitempty"`
}

// Publication is opt-in at the caller. A transport error is never a receipt.
func (g *GitLabRepository) PublishRunCheck(ctx context.Context, r Run, targetURL string, blocking bool, namespace string, authorize func(context.Context) error) CheckPublication {
	decoded, decodeErr := hex.DecodeString(namespace)
	if decodeErr != nil || len(namespace) != 32 || len(decoded) != 16 {
		return CheckPublication{State: "failed", Code: "invalid_identity"}
	}
	namespace = strings.ToLower(namespace)
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
	rules := effectiveCheckRules(r)
	if normalizeCheckRules(&rules) != nil {
		return CheckPublication{State: "failed", Code: "invalid_check_policy"}
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
	// Explicitly select the MR pipeline when GitLab supplies it. A pipeline
	// belonging to a merged-result SHA or another project cannot accept this
	// source-commit result without changing its meaning.
	if p := current.HeadPipeline; p != nil && (p.ID <= 0 || p.ProjectID != source || p.SHA != r.HeadSHA) {
		return CheckPublication{State: "failed", Code: "preflight_unavailable"}
	}
	if err := authorize(ctx); err != nil {
		return checkPreflightFailure(err)
	}
	a := assessRunCheck(r)
	rules.Mode = "advisory"
	if blocking {
		rules.Mode = "blocking"
	}
	state, thresholdCount := checkStateForRules(r, rules)
	// Keep one status per MR within an installation. A repeated audit must
	// replace the MR's result rather than leave a separate failed run job.
	name := fmt.Sprintf("aimangebot/%s/project/%d/mr/%d", namespace, r.ProjectID, r.MRIID)
	mode := "advisory"
	if blocking {
		mode = "blocking"
	}
	description := fmt.Sprintf("AIMergeBot run #%d: %s; risks>=%s: %d; mode=%s", r.ID, a.State, rules.MinimumSeverity, thresholdCount, mode)
	options := &gitlab.SetCommitStatusOptions{State: gitlab.BuildStateValue(state), Name: &name, Description: &description}
	if current.HeadPipeline != nil {
		options.PipelineID = &current.HeadPipeline.ID
	}
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
	if err := authorize(ctx); err != nil {
		outcome := checkPreflightFailure(err)
		// The POST has already happened. Losing authorization or latest-run
		// identity now cannot prove that no remote status was written.
		outcome.State = "unknown"
		outcome.RemoteID = receipt.ID
		return outcome
	}
	return CheckPublication{State: "published", RemoteID: receipt.ID}
}
