package platform

import (
	"context"
	"fmt"

	"github.com/xanzy/go-gitlab"
)

func (r *Runner) captureContextPolicy(ctx context.Context, project int, policy *AuditPolicy) (*AuditPolicy, error) {
	items, err := r.Store.ContextRepositories(ctx, project)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return policy, nil
	}
	copy := AuditPolicy{}
	if policy != nil {
		copy = *policy
	}
	copy.ContextRepositories = append([]ContextRepository{}, items...)
	return &copy, nil
}
func (r *Runner) authorizeContextRead(ctx context.Context, run Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.Store.OwnsRunningRun(ctx, run.ID, r.owner); err != nil {
		return err
	}
	if err := validateContextAdmission(ctx, r.Store.DB, run.Snapshot); err != nil {
		return err
	}
	if run.RequestedBy > 0 {
		_, err := requireSnapshotRole(ctx, r.Store.DB, run.Snapshot, run.RequestedBy, "operator")
		return err
	}
	return nil
}

func (r *Runner) prepareContextSources(ctx context.Context, run Run, gitConfig GitAuditSettings, existing map[int]ContextSource) (map[int]ContextSource, []string, func(), error) {
	sources := map[int]ContextSource{}
	notes := []string{}
	cleanups := []func(){}
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	if len(contextPolicyItems(run.Snapshot)) == 0 {
		return sources, notes, cleanup, nil
	}
	if err := r.authorizeContextRead(ctx, run); err != nil {
		return sources, notes, cleanup, err
	}
	var remote *GitLabRepository
	if r.Settings != nil {
		cfg := r.Settings.Snapshot()
		var err error
		remote, err = NewGitLabRepository(cfg.GitLab.Token, run.AuditPolicy.RepositoryURL)
		if err != nil {
			return sources, notes, cleanup, err
		}
	}
	for _, item := range contextPolicyItems(run.Snapshot) {
		snap := Snapshot{ProjectID: item.ProjectID, SourceProjectID: item.ProjectID, BaseSHA: item.SHA, HeadSHA: item.SHA}
		source := ContextSource{Snapshot: snap, Authorize: func(check context.Context) error { return r.authorizeContextRead(check, run) }}
		if remote != nil {
			commit, _, err := remote.Client.Commits.GetCommit(item.ProjectID, item.SHA, nil, gitlab.WithContext(ctx))
			if err == nil && commit != nil && commit.ID == item.SHA {
				source.Repository = remote
				if gitConfig.Enabled {
					local, stop, err := PrepareGitLab(ctx, remote, snap, gitConfig)
					if err == nil {
						source.Repository = local
						cleanups = append(cleanups, stop)
					} else {
						source.Repository = nil
					}
				}
			}
		} else if prepared, ok := existing[item.ProjectID]; ok && prepared.Snapshot.ProjectID == item.ProjectID && prepared.Snapshot.SourceProjectID == item.ProjectID && prepared.Snapshot.BaseSHA == item.SHA && prepared.Snapshot.HeadSHA == item.SHA {
			source.Repository = prepared.Repository
			previous := prepared.Authorize
			source.Authorize = func(check context.Context) error {
				if err := r.authorizeContextRead(check, run); err != nil {
					return err
				}
				if previous != nil {
					return previous(check)
				}
				return nil
			}
		}
		sources[item.ProjectID] = source
		if source.Repository == nil {
			notes = append(notes, fmt.Sprintf("Fixed context repository unavailable: project %d at %s", item.ProjectID, item.SHA))
		}
	}
	if err := r.authorizeContextRead(ctx, run); err != nil {
		cleanup()
		return sources, notes, func() {}, err
	}
	return sources, notes, cleanup, nil
}

func observationAtSnapshot(out toolOutput, snap Snapshot) bool {
	if out.RepositoryID == 0 {
		return out.BaseSHA == snap.BaseSHA && out.HeadSHA == snap.HeadSHA
	}
	for _, item := range contextPolicyItems(snap) {
		if item.ProjectID == out.RepositoryID && out.BaseSHA == item.SHA && out.HeadSHA == item.SHA {
			return true
		}
	}
	return false
}
