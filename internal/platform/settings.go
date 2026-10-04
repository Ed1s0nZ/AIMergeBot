package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
	legacy "pr_agent/internal"
)

// Settings retains the existing config format while introducing explicit platform controls.
type GitAuditSettings struct {
	Enabled      bool `yaml:"enabled" json:"enabled"`
	HistoryDepth int  `yaml:"history_depth" json:"history_depth"`
	MaxPackMiB   int  `yaml:"max_pack_mib" json:"max_pack_mib"`
	MaxToolCalls int  `yaml:"max_tool_calls" json:"max_tool_calls"`
}

func defaultGitAudit(c *GitAuditSettings) {
	if c.HistoryDepth == 0 {
		c.HistoryDepth = 200
	}
	if c.MaxPackMiB == 0 {
		c.MaxPackMiB = 256
	}
	if c.MaxToolCalls == 0 {
		c.MaxToolCalls = 80
	}
}

var ErrSettingsConflict = errors.New("settings changed; reload current configuration before saving")

type Settings struct {
	Revision                 uint64           `yaml:"config_revision" json:"config_revision"`
	VerifyFindings           bool             `yaml:"verify_findings" json:"verify_findings"`
	GenerateSequenceDiagrams bool             `yaml:"generate_sequence_diagrams" json:"generate_sequence_diagrams"`
	AuditQuotas              AuditQuotas      `yaml:"audit_quotas" json:"audit_quotas"`
	GitAudit                 GitAuditSettings `yaml:"git_audit" json:"git_audit"`
	legacy.Config            `yaml:",inline"`
	PublicURL                string   `yaml:"public_url" json:"public_url"`
	TrustedProxies           []string `yaml:"trusted_proxies" json:"trusted_proxies"`
	WebhookToken             string   `yaml:"webhook_token" json:"webhook_token"`
	AuditWorkers             int      `yaml:"audit_workers" json:"audit_workers"`
	AuditTimeoutSeconds      int      `yaml:"audit_timeout_seconds" json:"audit_timeout_seconds"`
}

type SettingsService struct {
	path    string
	mu      sync.Mutex
	current atomic.Pointer[Settings]
}

func OpenSettings(path, example string) (*SettingsService, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		raw, e := os.ReadFile(example)
		if e != nil {
			return nil, fmt.Errorf("read example config: %w", e)
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return nil, e
		}
		_, e = f.Write(raw)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Settings{GitAudit: GitAuditSettings{Enabled: true}, GenerateSequenceDiagrams: true, VerifyFindings: true}
	if err = yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.Revision == 0 {
		cfg.Revision = 1
	}
	defaultGitAudit(&cfg.GitAudit)
	defaultAuditQuotas(&cfg.AuditQuotas)
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.AuditWorkers == 0 {
		cfg.AuditWorkers = 2
	}
	if cfg.AuditTimeoutSeconds == 0 {
		cfg.AuditTimeoutSeconds = 300
	}
	if cfg.ReAct.MaxSteps == 0 {
		cfg.ReAct.MaxSteps = 16
	}
	if err = validateSettings(cfg); err != nil {
		return nil, err
	}
	svc := &SettingsService{path: path}
	svc.current.Store(&cfg)
	return svc, nil
}
func (s *SettingsService) Snapshot() Settings {
	cfg := *s.current.Load()
	cfg.TrustedProxies = append([]string{}, cfg.TrustedProxies...)
	cfg.WhitelistExtensions = append([]string{}, cfg.WhitelistExtensions...)
	cfg.Projects = append(cfg.Projects[:0:0], cfg.Projects...)
	for i := range cfg.Projects {
		if cfg.Projects[i].Enabled != nil {
			enabled := *cfg.Projects[i].Enabled
			cfg.Projects[i].Enabled = &enabled
		}
	}
	return cfg
}
func validURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func validateSettings(c Settings) error {
	defaultAuditQuotas(&c.AuditQuotas)
	if err := validateAuditQuotas(c.AuditQuotas); err != nil {
		return err
	}
	if err := validateTrustedProxies(c.TrustedProxies); err != nil {
		return err
	}
	defaultGitAudit(&c.GitAudit)
	if c.GitAudit.HistoryDepth < 1 || c.GitAudit.HistoryDepth > 10000 || c.GitAudit.MaxPackMiB < 16 || c.GitAudit.MaxPackMiB > 4096 || c.GitAudit.MaxToolCalls < 10 || c.GitAudit.MaxToolCalls > 200 {
		return fmt.Errorf("invalid Git audit limits")
	}
	_, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen must be host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("listen port must be 1–65535")
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || !validURL(c.PublicURL) || u.Path != "" && u.Path != "/" {
			return fmt.Errorf("public_url must be an HTTP(S) origin without a path")
		}
	}
	if c.AuditWorkers < 1 || c.AuditWorkers > 16 {
		return fmt.Errorf("audit_workers must be 1–16")
	}
	if c.AuditTimeoutSeconds < 10 || c.AuditTimeoutSeconds > 3600 {
		return fmt.Errorf("audit_timeout_seconds must be 10–3600")
	}
	if c.ReAct.MaxSteps < 2 || c.ReAct.MaxSteps > 100 {
		return fmt.Errorf("react.max_steps must be 2–100")
	}
	if c.ReAct.Temperature < 0 || c.ReAct.Temperature > 2 {
		return fmt.Errorf("temperature must be 0–2")
	}
	if !validURL(c.GitLab.URL) || !validURL(c.OpenAI.URL) {
		return fmt.Errorf("GitLab and model URLs must be absolute HTTP(S) URLs")
	}
	if c.Listen == "" {
		return fmt.Errorf("listen required")
	}
	return nil
}

// Save validates before an atomic replace; the live snapshot changes only after persistence succeeds.
func (s *SettingsService) Save(next Settings) error { return s.save(next, false) }

func (s *SettingsService) SyncProjects(projects []Project) error {
	next := s.Snapshot()
	next.Projects = []legacy.ProjectConfig{}
	for _, p := range projects {
		enabled := p.Enabled
		next.Projects = append(next.Projects, legacy.ProjectConfig{ID: p.ID, Name: p.Name, Enabled: &enabled})
	}
	return s.save(next, true)
}

func (s *SettingsService) save(next Settings, replaceProjects bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.Snapshot()
	if !replaceProjects && next.Revision != old.Revision {
		return ErrSettingsConflict
	}
	defaultGitAudit(&next.GitAudit)
	defaultAuditQuotas(&next.AuditQuotas)
	if next.GitLab.Token == "" {
		next.GitLab.Token = old.GitLab.Token
	}
	if next.OpenAI.APIKey == "" {
		next.OpenAI.APIKey = old.OpenAI.APIKey
	}
	if next.WebhookToken == "" {
		next.WebhookToken = old.WebhookToken
	}
	if replaceProjects {
		projects := next.Projects
		a, _ := json.Marshal(projects)
		b, _ := json.Marshal(old.Projects)
		if string(a) == string(b) {
			return nil
		}
		next = old
		next.Projects = projects
	} else {
		next.Projects = old.Projects
	} // project membership is changed through its dedicated API.
	if err := validateSettings(next); err != nil {
		return err
	}
	next.Revision = old.Revision + 1
	raw, err := yaml.Marshal(next)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".aim-config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, s.path); err != nil {
		return err
	}
	next.TrustedProxies = append([]string{}, next.TrustedProxies...)
	s.current.Store(&next)
	return nil
}
func (s *SettingsService) Public() map[string]any {
	cfg := s.Snapshot()
	cfg.GitLab.Token = ""
	cfg.OpenAI.APIKey = ""
	cfg.WebhookToken = ""
	// Expose the YAML names in JSON rather than legacy Go field names.
	raw, _ := yaml.Marshal(cfg)
	var out map[string]any
	_ = yaml.Unmarshal(raw, &out)
	real := s.Snapshot()
	out["has_gitlab_token"] = real.GitLab.Token != ""
	out["has_api_key"] = real.OpenAI.APIKey != ""
	out["has_webhook_token"] = real.WebhookToken != ""
	return out
}
func (s *SettingsService) DecodePublic(raw []byte) (Settings, error) {
	var mapping map[string]any
	if err := json.Unmarshal(raw, &mapping); err != nil {
		return Settings{}, err
	}
	delete(mapping, "has_gitlab_token")
	delete(mapping, "has_api_key")
	delete(mapping, "has_webhook_token")
	delete(mapping, "project_config_sync")
	delete(mapping, "restart_required")
	encoded, err := yaml.Marshal(mapping)
	if err != nil {
		return Settings{}, err
	}
	current := s.Snapshot()
	cfg := Settings{AuditQuotas: current.AuditQuotas, GitAudit: current.GitAudit, GenerateSequenceDiagrams: current.GenerateSequenceDiagrams, VerifyFindings: current.VerifyFindings, TrustedProxies: current.TrustedProxies}
	decoder := yaml.NewDecoder(strings.NewReader(string(encoded)))
	decoder.KnownFields(true)
	if err = decoder.Decode(&cfg); err != nil {
		return cfg, err
	}
	if _, present := mapping["audit_quotas"]; present {
		if err = validateAuditQuotas(cfg.AuditQuotas); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// DynamicRepository builds a client from the current persisted settings for each operation.
type DynamicRepository struct{ Settings *SettingsService }

func (d *DynamicRepository) repo() (*GitLabRepository, error) {
	s := d.Settings.Snapshot()
	return NewGitLabRepository(s.GitLab.Token, s.GitLab.URL)
}
func (d *DynamicRepository) Snapshot(ctx context.Context, p, i int) (Snapshot, error) {
	r, e := d.repo()
	if e != nil {
		return Snapshot{}, e
	}
	return r.Snapshot(ctx, p, i)
}
func (d *DynamicRepository) Changes(ctx context.Context, s Snapshot) ([]Change, []string, error) {
	r, e := d.repo()
	if e != nil {
		return nil, nil, e
	}
	return r.Changes(ctx, s)
}
func (d *DynamicRepository) ReadFile(ctx context.Context, s Snapshot, p string, b bool) (string, error) {
	r, e := d.repo()
	if e != nil {
		return "", e
	}
	return r.ReadFile(ctx, s, p, b)
}
func (d *DynamicRepository) ListFiles(ctx context.Context, s Snapshot, p int) ([]string, bool, error) {
	r, e := d.repo()
	if e != nil {
		return nil, false, e
	}
	return r.ListFiles(ctx, s, p)
}

type DynamicAuditor struct {
	Settings   *SettingsService
	Repository Repository
}

func (d *DynamicAuditor) Audit(ctx context.Context, s Snapshot, scope DiffScope) (AuditResult, []ToolTrace, error) {
	cfg := d.Settings.Snapshot()
	repo, e := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
	if e != nil {
		return AuditResult{}, nil, e
	}
	model := cfg.ReAct.Model
	if model == "" {
		model = cfg.OpenAI.Model
	}
	a := EinoAuditor{Repository: repo, Config: AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: cfg.ReAct.MaxSteps, Temperature: float32(cfg.ReAct.Temperature), MaxToolCalls: cfg.GitAudit.MaxToolCalls, GenerateDiagrams: cfg.GenerateSequenceDiagrams, VerifyFindings: cfg.VerifyFindings}}
	return a.Audit(ctx, s, scope)
}
