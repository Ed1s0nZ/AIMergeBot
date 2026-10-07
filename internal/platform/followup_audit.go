package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrFollowupScope = errors.New("follow-up scope unavailable or invalid; select changed files from a completed captured task")

type ScopeFile struct {
	Path       string `json:"path"`
	Status     string `json:"status"`
	Selectable bool   `json:"selectable"`
}
type RunScope struct {
	FollowupOf    int64       `json:"followup_of,omitempty"`
	SelectedFiles []string    `json:"selected_files"`
	BaseSHA       string      `json:"base_sha"`
	HeadSHA       string      `json:"head_sha"`
	Files         []ScopeFile `json:"files"`
	Notes         []string    `json:"notes"`
	Truncated     bool        `json:"truncated"`
}

func normalizeSelectedFiles(files []string) ([]string, error) {
	if len(files) < 1 || len(files) > 100 {
		return nil, ErrFollowupScope
	}
	seen := map[string]bool{}
	out := []string{}
	size := 0
	for _, p := range files {
		if !validPath(p) || len(p) > 1024 || strings.ContainsAny(p, "\r\n\x00") {
			return nil, ErrFollowupScope
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		size += len(p)
		if size > 16*1024 {
			return nil, ErrFollowupScope
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
func selectAuditChanges(changes []Change, files []string) ([]Change, []string, error) {
	if len(files) == 0 {
		return changes, nil, nil
	}
	wanted := map[string]bool{}
	for _, p := range files {
		wanted[p] = true
	}
	out := []Change{}
	outside := []string{}
	for _, c := range changes {
		p := changePath(c)
		if wanted[p] {
			out = append(out, c)
			delete(wanted, p)
		} else {
			outside = append(outside, p)
		}
	}
	if len(wanted) > 0 {
		return nil, nil, ErrFollowupScope
	}
	return out, outside, nil
}
func makeRunScope(run Run, changes []Change, notes []string) RunScope {
	p := run.AuditPolicy
	out := RunScope{BaseSHA: run.BaseSHA, HeadSHA: run.HeadSHA, SelectedFiles: []string{}, Files: []ScopeFile{}, Notes: append([]string{}, notes...)}
	if p == nil {
		return out
	}
	out.FollowupOf = p.FollowupOf
	out.SelectedFiles = append(out.SelectedFiles, p.SelectedFiles...)
	selected, _, err := selectAuditChanges(changes, p.SelectedFiles)
	if err != nil {
		out.Notes = append(out.Notes, "Selected snapshot files unavailable")
		selected = nil
	}
	plan := PlanAuditGroups(selected, p.Excluded)
	included := map[string]bool{}
	excluded := map[string]bool{}
	selectedPaths := map[string]bool{}
	for _, c := range selected {
		selectedPaths[changePath(c)] = true
	}
	for _, file := range plan.Excluded {
		excluded[file] = true
	}
	whole := BuildDiff(selected, p.Excluded, 96*1024)
	useGroups := len(plan.Groups) > 1 || whole.Text == "" && len(plan.Groups) > 0
	if useGroups {
		for _, g := range plan.Groups {
			for _, file := range g.Files {
				included[file] = true
			}
		}
		out.Notes = append(out.Notes, plan.Notes...)
	} else {
		scope := whole
		for _, file := range scope.Included {
			included[file] = true
		}
		out.Notes = append(out.Notes, scope.Notes...)
	}
	omitted := map[string]bool{}
	if useGroups {
		for _, file := range plan.OmittedFiles {
			omitted[file] = true
		}
	}
	groupStatus := map[string]string{}
	for _, g := range run.Result.AuditGroups {
		for _, file := range g.Files {
			previous := groupStatus[file]
			if previous == "failed" || g.Status == "failed" {
				groupStatus[file] = "failed"
			} else if previous == "unprocessed" || previous == "running" || g.Status == "unprocessed" || g.Status == "running" {
				groupStatus[file] = "unprocessed"
			} else {
				groupStatus[file] = g.Status
			}
		}
	}
	for _, c := range changes {
		path := changePath(c)
		// Eligibility always applies the parent's exclusion policy, including files
		// outside an earlier follow-up selection.
		single := BuildDiff([]Change{c}, p.Excluded, 96*1024)
		blocked := len(single.Excluded) > 0 || !validPath(path)
		status := "not_in_input"
		if included[path] {
			status = "included"
			if omitted[path] {
				status = "partial_input"
			}
			if groupStatus[path] == "failed" {
				status = "group_failed"
			}
			if groupStatus[path] == "unprocessed" || groupStatus[path] == "running" {
				status = "group_unprocessed"
			}
		}
		if !selectedPaths[path] && len(p.SelectedFiles) > 0 {
			status = "outside_scope"
		}
		if blocked || excluded[path] {
			status = "excluded"
		}
		if len(out.Files) >= 5000 {
			out.Truncated = true
			continue
		}
		out.Files = append(out.Files, ScopeFile{Path: path, Status: status, Selectable: !blocked})
	}
	if len(out.Notes) > 200 {
		out.Notes = append(out.Notes[:200], "Coverage notes truncated in scope view; original audit evidence is retained")
		out.Truncated = true
	}
	return out
}
func (r *Runner) pinnedScopeChanges(ctx context.Context, run Run) ([]Change, []string, error) {
	if err := requireLegacyRepositorySnapshot(ctx, r.Store.DB, run.Snapshot); err != nil {
		return nil, nil, err
	}
	if run.AuditPolicy == nil {
		return nil, nil, ErrFollowupScope
	}
	repo := r.Repository
	cleanup := func() {}
	if r.Settings != nil {
		cfg := r.Settings.Snapshot()
		if strings.TrimRight(cfg.GitLab.URL, "/") != run.AuditPolicy.RepositoryURL {
			return nil, nil, ErrFollowupScope
		}
		remote, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
		if err != nil {
			return nil, nil, err
		}
		repo = remote
		if run.AuditPolicy.Git.Enabled {
			local, stop, err := PrepareGitLab(ctx, remote, run.Snapshot, run.AuditPolicy.Git)
			if err != nil {
				return nil, nil, err
			}
			repo = local
			cleanup = stop
		}
	}
	defer cleanup()
	if repo == nil {
		return nil, nil, ErrFollowupScope
	}
	return repo.Changes(ctx, run.Snapshot)
}
func (r *Runner) Scope(ctx context.Context, id, actor int64) (RunScope, error) {
	run, err := r.Store.Run(ctx, id)
	if err != nil {
		return RunScope{}, err
	}
	if _, err = requireSnapshotRole(ctx, r.Store.DB, run.Snapshot, actor, "viewer"); err != nil {
		return RunScope{}, err
	}
	changes, notes, err := r.pinnedScopeChanges(ctx, run)
	if err != nil {
		return RunScope{}, err
	}
	return makeRunScope(run, changes, notes), nil
}
func (r *Runner) SubmitFollowup(ctx context.Context, parentID, actor int64, files []string) (int64, bool, error) {
	if r.workerStopped() {
		return 0, false, ErrWorkerLeaseLost
	}
	if actor <= 0 {
		return 0, false, ErrCredentials
	}
	selected, err := normalizeSelectedFiles(files)
	if err != nil {
		return 0, false, err
	}
	parent, err := r.Store.Run(ctx, parentID)
	if err != nil {
		return 0, false, err
	}
	if _, err = requireSnapshotRole(ctx, r.Store.DB, parent.Snapshot, actor, "operator"); err != nil {
		return 0, false, err
	}
	if parent.Status == "pending" || parent.Status == "running" || parent.AuditPolicy == nil {
		return 0, false, ErrFollowupScope
	}
	var enabled bool
	if err = r.Store.DB.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, parent.ProjectID).Scan(&enabled); err != nil {
		return 0, false, err
	}
	if !enabled {
		return 0, false, ErrConflict
	}
	changes, _, err := r.pinnedScopeChanges(ctx, parent)
	if err != nil {
		return 0, false, err
	}
	scoped, _, err := selectAuditChanges(changes, selected)
	if err != nil {
		return 0, false, err
	}
	for _, c := range scoped {
		if len(BuildDiff([]Change{c}, parent.AuditPolicy.Excluded, 96*1024).Excluded) > 0 {
			return 0, false, ErrFollowupScope
		}
	}
	policy := *parent.AuditPolicy
	policy.FollowupOf = parentID
	policy.SelectedFiles = selected
	snapshot := parent.Snapshot
	snapshot.AuditPolicy = &policy
	return r.Store.EnqueueUser(ctx, snapshot, actor, true)
}
func followupCoverageNote(p *AuditPolicy) string {
	return fmt.Sprintf("Selected-file follow-up of run %d; %d selected changed files only. Other files are context, not audited scope.", p.FollowupOf, len(p.SelectedFiles))
}

// Recheck mutable project/parent state in the admission transaction after Git
// retrieval, so a project disabled while fetching cannot admit a follow-up.
func validateFollowupEnqueue(ctx context.Context, tx *sql.Tx, snap Snapshot) error {
	p := snap.AuditPolicy
	if p == nil {
		return nil
	}
	if p.FollowupOf == 0 && len(p.SelectedFiles) == 0 {
		return nil
	}
	if p.FollowupOf <= 0 {
		return ErrFollowupScope
	}
	files, err := normalizeSelectedFiles(p.SelectedFiles)
	if err != nil {
		return err
	}
	if len(files) != len(p.SelectedFiles) {
		return ErrFollowupScope
	}
	for i := range files {
		if files[i] != p.SelectedFiles[i] {
			return ErrFollowupScope
		}
	}
	var parent Snapshot
	var status, policyRaw string
	var enabled bool
	err = tx.QueryRowContext(ctx, `SELECT r.project_id,r.source_project_id,r.mr_iid,r.diff_version_id,r.base_sha,r.head_sha,r.status,r.audit_policy_json,p.enabled FROM platform_runs r JOIN platform_projects p ON p.id=r.project_id WHERE r.id=?`, p.FollowupOf).Scan(&parent.ProjectID, &parent.SourceProjectID, &parent.MRIID, &parent.DiffVersionID, &parent.BaseSHA, &parent.HeadSHA, &status, &policyRaw, &enabled)
	if err != nil {
		return err
	}
	if !enabled || status == "pending" || status == "running" {
		return ErrConflict
	}
	if parent.ProjectID != snap.ProjectID || parent.SourceProjectID != snap.SourceProjectID || parent.MRIID != snap.MRIID || parent.DiffVersionID != snap.DiffVersionID || parent.BaseSHA != snap.BaseSHA || parent.HeadSHA != snap.HeadSHA {
		return ErrFollowupScope
	}
	if err = json.Unmarshal([]byte(policyRaw), &parent.AuditPolicy); err != nil || parent.AuditPolicy == nil {
		return ErrFollowupScope
	}
	inherited := *parent.AuditPolicy
	candidate := *p
	inherited.FollowupOf = 0
	inherited.SelectedFiles = nil
	candidate.FollowupOf = 0
	candidate.SelectedFiles = nil
	if policyDigest(&inherited) != policyDigest(&candidate) {
		return ErrFollowupScope
	}
	return nil
}
