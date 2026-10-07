package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Runner struct {
	Settings    *SettingsService
	Store       *Store
	Repository  Repository
	Auditor     Auditor
	Workers     int
	Timeout     time.Duration
	Excluded    []string
	mu          sync.Mutex
	active      map[int64]context.CancelFunc
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	lifecycleMu sync.Mutex
	owner       string
	state       atomic.Pointer[workerRunState]
	failures    chan error
}

func (r *Runner) Start(parent context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.cancel != nil {
		return fmt.Errorf("runner already started")
	}
	owner, err := newWorkerOwner()
	if err != nil {
		return err
	}
	if err = r.Store.AcquireWorkerInstance(parent, owner); err != nil {
		return err
	}
	r.owner = owner

	if r.Settings != nil {
		r.Store.BindQuotaSettings(r.Settings)
	}
	if err := r.Store.Recover(parent); err != nil {
		r.releaseLease()
		r.owner = ""
		return err
	}
	if err = r.Store.RenewWorkerInstance(parent, owner); err != nil {
		r.releaseLease()
		r.owner = ""
		return err
	}
	if r.Workers <= 0 {
		r.Workers = 2
	}
	if r.Workers > 16 {
		r.Workers = 16
	}
	if r.Timeout <= 0 {
		r.Timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.failures = make(chan error, 1)
	r.state.Store(&workerRunState{ctx: ctx, failures: r.failures, owner: owner})
	r.mu.Lock()
	r.active = map[int64]context.CancelFunc{}
	r.mu.Unlock()
	r.wg.Add(1)
	go func() { defer r.wg.Done(); r.leaseLoop(ctx) }()
	r.wg.Add(1)
	go func() { defer r.wg.Done(); r.notificationLoop(ctx) }()
	for i := 0; i < r.Workers; i++ {
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.loop(ctx) }()
	}
	if r.Settings != nil {
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.checkLoop(ctx) }()
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.commentLoop(ctx) }()
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.Poll(ctx) }()
	}
	return nil
}
func (r *Runner) Stop() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
	r.releaseLease()
	r.cancel = nil
	r.owner = ""

}
func (r *Runner) Submit(ctx context.Context, pid, iid int, actor int64, force bool) (int64, bool, error) {
	if r.workerStopped() {
		return 0, false, ErrWorkerLeaseLost
	}
	if actor > 0 {
		if _, err := requireProjectRole(ctx, r.Store.DB, pid, actor, "operator"); err != nil {
			return 0, false, err
		}
	}
	var enabled bool
	if err := r.Store.DB.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, pid).Scan(&enabled); err != nil {
		return 0, false, err
	}
	if !enabled {
		return 0, false, errors.New("project disabled")
	}
	repository := r.Repository
	var policy *AuditPolicy
	if r.Settings != nil {
		cfg := r.Settings.Snapshot()
		policy = capturePolicy(cfg)
		pinned, e := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
		if e != nil {
			return 0, false, e
		}
		repository = pinned
	}
	snap, err := repository.Snapshot(ctx, pid, iid)
	if err != nil {
		return 0, false, err
	}
	policy, err = r.captureContextPolicy(ctx, pid, policy)
	if err != nil {
		return 0, false, err
	}
	workflow, err := readWorkflowPolicy(ctx, r.Store.DB, pid)
	if err != nil {
		return 0, false, err
	}
	if workflow.Revision > 0 {
		if policy == nil {
			policy = &AuditPolicy{Excluded: append([]string{}, r.Excluded...)}
		}
		policy.Workflow = &workflow
		policy.Excluded = append(policy.Excluded, workflow.ExcludedExtensions...)
	}
	snap.AuditPolicy = policy
	if actor > 0 {
		return r.Store.EnqueueUser(ctx, snap, actor, force)
	}
	return r.Store.Enqueue(ctx, snap, actor, force)
}
func (r *Runner) Cancel(ctx context.Context, id, actor int64) error {
	var err error
	if actor > 0 {
		err = r.Store.CancelUser(ctx, id, actor)
	} else {
		err = r.Store.Cancel(ctx, id, actor)
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	if cancel := r.active[id]; cancel != nil {
		cancel()
	}
	r.mu.Unlock()
	return nil
}
func (r *Runner) loop(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		id, err := r.Store.ClaimOwned(ctx, r.owner)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("audit queue claim failed: %v", err)
			}
			continue
		}
		r.execute(ctx, id)
	}
}
func (r *Runner) execute(parent context.Context, id int64) {
	leaseCtx, leaseCancel := context.WithCancel(parent)
	defer leaseCancel()
	parent = leaseCtx
	r.mu.Lock()
	if r.active == nil {
		r.active = map[int64]context.CancelFunc{}
	}
	r.active[id] = leaseCancel
	r.mu.Unlock()
	defer r.releaseAttemptLease(id)
	if err := r.Store.OwnsRunningRun(parent, id, r.owner); err != nil {
		return
	}
	defer func() {
		if recover() != nil {
			failureCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			if !r.workerStopped() {
				_ = r.Store.FailWorker(failureCtx, id, r.owner, "unexpected worker failure; checkpoint retained")
			}
			stop()
			log.Printf("audit run %d recovered a worker failure", id)
		}
	}()
	run, err := r.Store.Run(parent, id)
	if err != nil {
		r.finish(id, "failed", "unable to load run", AuditResult{}, nil)
		return
	}
	if run.RequestedBy > 0 {
		if _, err = requireSnapshotRole(parent, r.Store.DB, run.Snapshot, run.RequestedBy, "operator"); err != nil {
			if errors.Is(err, ErrCredentials) || errors.Is(err, ErrProjectPermission) || errors.Is(err, sql.ErrNoRows) {
				r.finish(id, "cancelled", "requesting user's project execution access revoked", run.Result, run.Trace)
			} else {
				r.finish(id, "failed", "unable to check project execution access", run.Result, run.Trace)
			}
			return
		}
	}
	var gitConfig GitAuditSettings
	timeout := r.Timeout
	repo, auditor, excluded := r.Repository, r.Auditor, r.Excluded
	if run.AuditPolicy != nil {
		excluded = run.AuditPolicy.Excluded
	}
	if r.Settings != nil {
		cfg := r.Settings.Snapshot()
		if p := run.AuditPolicy; p != nil {
			if capturePolicy(cfg).RepositoryURL != p.RepositoryURL || cfg.OpenAI.URL != p.ModelURL {
				r.finish(id, "failed", "service endpoint changed since enqueue; resubmit audit to bind current credentials", AuditResult{}, nil)
				return
			}
			cfg.GitLab.URL = p.RepositoryURL
			cfg.OpenAI.URL = p.ModelURL
			cfg.OpenAI.Model = p.Model
			cfg.ReAct.Model = p.Model
			cfg.ReAct.Temperature = float64(p.Temperature)
			cfg.ReAct.MaxSteps = p.MaxSteps
			cfg.ModelBudget = p.ModelBudget
			cfg.VerificationModel = p.VerificationModel
			cfg.AuditTimeoutSeconds = p.TimeoutSeconds
			cfg.GitAudit = p.Git
			cfg.WhitelistExtensions = p.Excluded
			cfg.GenerateSequenceDiagrams = p.GenerateDiagrams
			cfg.VerifyFindings = p.VerifyFindings
		}
		gitConfig = cfg.GitAudit
		timeout = time.Duration(cfg.AuditTimeoutSeconds) * time.Second
		excluded = cfg.WhitelistExtensions
		pinned, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
		if err != nil {
			r.finish(id, "failed", "invalid repository configuration", AuditResult{}, nil)
			return
		}
		repo = pinned
		model := cfg.ReAct.Model
		if model == "" {
			model = cfg.OpenAI.Model
		}
		auditor = &EinoAuditor{Repository: pinned, Config: AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: cfg.ReAct.MaxSteps, MaxTokens: cfg.ModelBudget.MaxTokens, VerificationModel: cfg.VerificationModel, Temperature: float32(cfg.ReAct.Temperature), MaxToolCalls: cfg.GitAudit.MaxToolCalls, GenerateDiagrams: cfg.GenerateSequenceDiagrams, VerifyFindings: cfg.VerifyFindings}}
	}
	if original, ok := auditor.(*EinoAuditor); ok {
		copy := *original
		copy.Config.Progress = func(result AuditResult, trace []ToolTrace) error {
			checkpointCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			return r.Store.CheckpointOwned(checkpointCtx, id, r.owner, result, trace)
		}
		auditor = &copy
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	run, err = r.Store.Run(ctx, id)
	if err != nil {
		r.finish(id, "failed", "unable to load run", AuditResult{}, nil)
		return
	}
	if run.Status != "running" {
		return
	}
	if run.PolicyVersion != PolicyVersion {
		r.finish(id, "failed", "audit policy changed; submit a new audit", AuditResult{}, nil)
		return
	}
	ctx, err = r.Store.seedRetryModelBudget(ctx, run)
	if err != nil {
		r.finish(id, "incomplete", "", budgetInterruptionResult(err), run.Trace)
		return
	}
	existingSources := map[int]ContextSource{}
	if prepared, ok := auditor.(*EinoAuditor); ok {
		existingSources = prepared.ContextSources
	}
	sources, contextNotes, contextCleanup, err := r.prepareContextSources(ctx, run, gitConfig, existingSources)
	if err != nil {
		r.finish(id, "incomplete", "", AuditResult{Summary: "Fixed context preparation interrupted", CoverageNotes: []string{"Fixed context preparation interrupted; check authorization, availability and task timeout before resubmitting"}}, run.Trace)
		return
	}
	defer contextCleanup()
	if prepared, ok := auditor.(*EinoAuditor); ok {
		copy := *prepared
		copy.ContextSources = sources
		auditor = &copy
	} else if len(sources) > 0 {
		r.finish(id, "incomplete", "", AuditResult{Summary: "Context-aware auditor required", CoverageNotes: []string{"Configured context repositories could not be read by this auditor"}}, nil)
		return
	}
	if gitConfig.Enabled {
		remote, ok := repo.(*GitLabRepository)
		if !ok {
			r.finish(id, "failed", "native Git preparation requires GitLab repository metadata", AuditResult{}, nil)
			return
		}
		local, cleanup, e := PrepareGitLab(ctx, remote, run.Snapshot, gitConfig)
		if e != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			if r.failTransient(id, e, AuditResult{}, nil) {
				return
			}
			r.finish(id, "failed", "cannot prepare pinned Git repository: "+e.Error(), AuditResult{}, nil)
			return
		}
		defer cleanup()
		repo = local
		if original, ok := auditor.(*EinoAuditor); ok {
			copy := *original
			copy.Repository = local
			auditor = &copy
		}
	}
	changes, notes, err := repo.Changes(ctx, run.Snapshot)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		if r.failTransient(id, err, AuditResult{}, nil) {
			return
		}
		r.finish(id, "failed", "cannot obtain pinned diff: "+err.Error(), AuditResult{}, nil)
		return
	}
	notes = append(notes, contextNotes...)
	if p := run.AuditPolicy; p != nil && len(p.SelectedFiles) > 0 {
		changes, _, err = selectAuditChanges(changes, p.SelectedFiles)
		if err != nil {
			r.finish(id, "incomplete", "selected snapshot scope unavailable", AuditResult{CoverageNotes: []string{"Selected snapshot files unavailable"}}, nil)
			return
		}
		notes = append(notes, followupCoverageNote(p))
	}
	plan := PlanAuditGroups(changes, excluded)
	grouped, canGroup := auditor.(*EinoAuditor)
	scope := BuildDiff(changes, excluded, 96*1024)
	useGroups := canGroup && (len(plan.Groups) > 1 || scope.Text == "" && len(plan.Groups) > 0)
	scope.Notes = append(scope.Notes, notes...)
	if scope.Text == "" && !useGroups {
		status := "incomplete"
		if len(scope.Excluded) > 0 && len(scope.Notes) == 0 {
			status = "skipped"
		}
		r.finish(id, status, "", AuditResult{Summary: "No auditable textual changes in configured scope", CoverageNotes: scope.Notes, ExcludedFiles: scope.Excluded}, nil)
		return
	}
	var result AuditResult
	var trace []ToolTrace
	if useGroups {
		plan.Notes = append(plan.Notes, notes...)
		result, trace, err = grouped.AuditGroups(ctx, run.Snapshot, plan)
	} else {
		result, trace, err = auditor.Audit(ctx, run.Snapshot, scope)
	}
	if run.AuditPolicy != nil && run.AuditPolicy.Workflow != nil && run.AuditPolicy.Workflow.FormatNoiseHints {
		result.FormatHints = formattingHints(changes, scope.Included)
	}
	status, message := "succeeded", ""
	if err != nil {
		status, message = "failed", err.Error()
		if auditCoverageStop(err) {
			status = "incomplete"
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status, message = "incomplete", "task timeout exceeded"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		// User cancellations already changed durable state. Service interruption
		// leaves checkpoints running until its leases expire for bounded recovery.
		return
	}
	if err == nil && len(result.CoverageNotes) > 0 {
		status = "incomplete"
	}
	if status == "failed" && ctx.Err() == nil && r.failTransient(id, err, result, trace) {
		return
	}
	// Successful completion schedules publication in the same database write.
	r.finish(id, status, message, result, trace)
}
func (r *Runner) finish(id int64, status, message string, result AuditResult, trace []ToolTrace) bool {
	if r.workerStopped() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Store.FinishOwned(ctx, id, r.owner, status, message, result, trace); err != nil {
		if !errors.Is(err, ErrConflict) {
			log.Printf("audit run %d persistence failed: %v", id, err)
		}
		return false
	}
	return true
}
