package platform

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// AuditGit runs the same language-independent Agent without GitLab metadata or comments.
// Local refs are resolved to immutable commits; remote mode requires full commit IDs.
func AuditGit(ctx context.Context, location, base, head, token string, cfg Settings) (Run, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.AuditTimeoutSeconds)*time.Second)
	defer cancel()
	snap := Snapshot{BaseSHA: base, HeadSHA: head, Title: "Standalone Git audit"}
	var repo *GitRepository
	u, e := url.Parse(location)
	if e != nil {
		return Run{}, fmt.Errorf("invalid repository location")
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		r, cleanup, e := PrepareRemoteGit(ctx, location, token, snap, cfg.GitAudit)
		if e != nil {
			return Run{}, e
		}
		defer cleanup()
		repo = r
	} else {
		if u.Scheme != "" {
			return Run{}, fmt.Errorf("use local repository path or HTTP(S) clone URL")
		}
		dir, e := filepath.Abs(location)
		if e != nil {
			return Run{}, e
		}
		repo = &GitRepository{Directory: dir}
		for _, target := range []*string{&snap.BaseSHA, &snap.HeadSHA} {
			if *target == "" {
				return Run{}, fmt.Errorf("base/head required")
			}
			raw, e := repo.command(ctx, "rev-parse", "--verify", "--end-of-options", *target+"^{commit}")
			if e != nil {
				return Run{}, e
			}
			*target = strings.TrimSpace(raw)
			if !commitID.MatchString(*target) {
				return Run{}, fmt.Errorf("cannot resolve commit")
			}
		}
		raw, e := repo.command(ctx, "rev-parse", "--is-shallow-repository")
		if e != nil {
			return Run{}, e
		}
		repo.HistoryLimited = strings.TrimSpace(raw) == "true"
	}
	changes, notes, e := repo.Changes(ctx, snap)
	if e != nil {
		return Run{}, e
	}
	scope := BuildDiff(changes, cfg.WhitelistExtensions, 96*1024)
	scope.Notes = append(scope.Notes, notes...)
	plan := PlanAuditGroups(changes, cfg.WhitelistExtensions)
	useGroups := len(plan.Groups) > 1 || scope.Text == "" && len(plan.Groups) > 0
	run := Run{Snapshot: snap, PolicyVersion: PolicyVersion, Status: "incomplete"}
	if scope.Text == "" && !useGroups {
		if len(scope.Excluded) > 0 && len(scope.Notes) == 0 {
			run.Status = "skipped"
		}
		run.Result = AuditResult{Summary: "No auditable textual changes", Findings: []Finding{}, CoverageNotes: scope.Notes, ExcludedFiles: scope.Excluded}
		return run, nil
	}
	model := cfg.ReAct.Model
	if model == "" {
		model = cfg.OpenAI.Model
	}
	auditor := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: cfg.ReAct.MaxSteps, MaxTokens: cfg.ModelBudget.MaxTokens, VerificationModel: cfg.VerificationModel, Temperature: float32(cfg.ReAct.Temperature), MaxToolCalls: cfg.GitAudit.MaxToolCalls, GenerateDiagrams: cfg.GenerateSequenceDiagrams, VerifyFindings: cfg.VerifyFindings}}
	if useGroups {
		plan.Notes = append(plan.Notes, notes...)
		run.Result, run.Trace, e = auditor.AuditGroups(ctx, snap, plan)
	} else {
		run.Result, run.Trace, e = auditor.Audit(ctx, snap, scope)
	}
	if e != nil {
		run.Status = "failed"
		if auditCoverageStop(e) {
			run.Status = "incomplete"
		}
		run.Error = e.Error()
		return run, e
	}
	if len(run.Result.CoverageNotes) == 0 {
		run.Status = "succeeded"
	}
	return run, nil
}
