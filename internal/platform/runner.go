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
	var enabled bool
	if err := r.Store.DB.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, pid).Scan(&enabled); err != nil {
		return 0, false, err
	}
	if !enabled {
		return 0, false, errors.New("project disabled")
	}
	snap, err := r.Repository.Snapshot(ctx, pid, iid)
	if err != nil {
		return 0, false, err
	}
	return r.Store.Enqueue(ctx, snap, actor, force)
}
func (r *Runner) Cancel(ctx context.Context, id, actor int64) error {
	if err := r.Store.Cancel(ctx, id, actor); err != nil {
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
			r.finish(id, "failed", "unexpected worker failure", AuditResult{}, nil)
			log.Printf("audit run %d recovered a worker failure", id)
		}
	}()
	var gitConfig GitAuditSettings
	timeout := r.Timeout
	repo, auditor, excluded := r.Repository, r.Auditor, r.Excluded
	if r.Settings != nil {
		cfg := r.Settings.Snapshot()
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
		auditor = &EinoAuditor{Repository: pinned, Config: AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: cfg.ReAct.MaxSteps, Temperature: float32(cfg.ReAct.Temperature), MaxToolCalls: cfg.GitAudit.MaxToolCalls}}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	r.mu.Lock()
	r.active[id] = cancel
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.active, id); r.mu.Unlock() }()
	run, err := r.Store.Run(ctx, id)
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
		r.finish(id, "failed", "cannot obtain pinned diff: "+err.Error(), AuditResult{}, nil)
		return
	}
	scope := BuildDiff(changes, excluded, 96*1024)
	scope.Notes = append(scope.Notes, notes...)
	if scope.Text == "" {
		r.finish(id, "incomplete", "no auditable textual changes", AuditResult{Summary: "No auditable textual changes", CoverageNotes: scope.Notes}, nil)
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
