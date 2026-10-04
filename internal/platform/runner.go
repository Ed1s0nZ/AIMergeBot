package platform

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"time"
)

type Runner struct {
	Settings   *SettingsService
	Store      *Store
	Repository Repository
	Auditor    Auditor
	Workers    int
	Timeout    time.Duration
	Excluded   []string
	mu         sync.Mutex
	active     map[int64]context.CancelFunc
	wg         sync.WaitGroup
	cancel     context.CancelFunc
}

func (r *Runner) Start(parent context.Context) error {
	if r.Settings != nil {
		r.Store.BindQuotaSettings(r.Settings)
	}
	if err := r.Store.Recover(parent); err != nil {
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
	r.active = map[int64]context.CancelFunc{}
	for i := 0; i < r.Workers; i++ {
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.loop(ctx) }()
	}
	if r.Settings != nil {
		r.wg.Add(1)
		go func() { defer r.wg.Done(); r.Poll(ctx) }()
	}
	return nil
}
func (r *Runner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}
func (r *Runner) Submit(ctx context.Context, pid, iid int, actor int64, force bool) (int64, bool, error) {
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
		id, err := r.Store.Claim(ctx)
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
	defer func() {
		if recover() != nil {
			failureCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = r.Store.DB.ExecContext(failureCtx, `UPDATE platform_runs SET status='failed',error='unexpected worker failure; checkpoint retained',finished_at=? WHERE id=? AND status='running'`, now(), id)
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
			cfg.AuditTimeoutSeconds = p.TimeoutSeconds
			cfg.GitAudit = p.Git
			cfg.WhitelistExtensions = p.Excluded
			cfg.GenerateSequenceDiagrams = p.GenerateDiagrams
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
		auditor = &EinoAuditor{Repository: pinned, Config: AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: cfg.ReAct.MaxSteps, Temperature: float32(cfg.ReAct.Temperature), MaxToolCalls: cfg.GitAudit.MaxToolCalls, GenerateDiagrams: cfg.GenerateSequenceDiagrams}}
	}
	if original, ok := auditor.(*EinoAuditor); ok {
		copy := *original
		copy.Config.Progress = func(result AuditResult, trace []ToolTrace) error {
			checkpointCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			return r.Store.Checkpoint(checkpointCtx, id, result, trace)
		}
		auditor = &copy
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	r.mu.Lock()
	r.active[id] = cancel
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.active, id); r.mu.Unlock() }()
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
	if gitConfig.Enabled {
		remote, ok := repo.(*GitLabRepository)
		if !ok {
			r.finish(id, "failed", "native Git preparation requires GitLab repository metadata", AuditResult{}, nil)
			return
		}
		local, cleanup, e := PrepareGitLab(ctx, remote, run.Snapshot, gitConfig)
		if e != nil {
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
		if r.failTransient(id, err, AuditResult{}, nil) {
			return
		}
		r.finish(id, "failed", "cannot obtain pinned diff: "+err.Error(), AuditResult{}, nil)
		return
	}
	scope := BuildDiff(changes, excluded, 96*1024)
	scope.Notes = append(scope.Notes, notes...)
	if scope.Text == "" {
		status := "incomplete"
		if len(scope.Excluded) > 0 && len(scope.Notes) == 0 {
			status = "skipped"
		}
		r.finish(id, status, "", AuditResult{Summary: "No auditable textual changes in configured scope", CoverageNotes: scope.Notes, ExcludedFiles: scope.Excluded}, nil)
		return
	}
	result, trace, err := auditor.Audit(ctx, run.Snapshot, scope)
	status, message := "succeeded", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status, message = "incomplete", "task timeout exceeded"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		status, message = "failed", "service interrupted task"
	}
	if err == nil && len(result.CoverageNotes) > 0 {
		status = "incomplete"
	}
	if status == "failed" && ctx.Err() == nil && r.failTransient(id, err, result, trace) {
		return
	}
	r.finish(id, status, message, result, trace)
	if status == "succeeded" {
		r.comment(ctx, id)
	}
}
func (r *Runner) finish(id int64, status, message string, result AuditResult, trace []ToolTrace) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Store.Finish(ctx, id, status, message, result, trace); err != nil && !errors.Is(err, ErrConflict) {
		log.Printf("audit run %d persistence failed: %v", id, err)
	}
}
