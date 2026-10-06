package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/cloudwego/eino/compose"
	modelopenai "github.com/meguminnnnnnnnn/go-openai"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pr_agent/internal/evaluation"
	"pr_agent/internal/platform"
)

type metadata struct {
	Grouped           bool                         `json:"grouped"`
	MaxTokens         int                          `json:"max_tokens"`
	VerificationModel string                       `json:"verification_model"`
	ModelBudget       platform.ModelBudgetSettings `json:"model_budget"`
	OnlyCase          string                       `json:"only_case,omitempty"`
	CorpusDigest      string                       `json:"corpus_digest"`
	CodeRevision      string                       `json:"code_revision"`
	Model             string                       `json:"model"`
	EndpointDigest    string                       `json:"endpoint_digest"`
	Policy            string                       `json:"policy"`
	MaxSteps          int                          `json:"max_steps"`
	MaxToolCalls      int                          `json:"max_tool_calls"`
	Temperature       float32                      `json:"temperature"`
	TimeoutSeconds    int                          `json:"timeout_seconds"`
	Verify            bool                         `json:"verify_findings"`
	Diagrams          bool                         `json:"generate_diagrams"`
}
type receipt struct {
	ContextRepositories   []platform.ContextRepository `json:"context_repositories,omitempty"`
	Usage                 platform.ModelUsage          `json:"usage"`
	ErrorClass            string                       `json:"error_class,omitempty"`
	HTTPStatus            int                          `json:"http_status,omitempty"`
	ID                    string                       `json:"id"`
	BaseSHA               string                       `json:"base_sha"`
	HeadSHA               string                       `json:"head_sha"`
	Status                string                       `json:"status"`
	ElapsedMS             int64                        `json:"elapsed_ms"`
	Result                platform.AuditResult         `json:"result"`
	Trace                 []platform.ToolTrace         `json:"trace"`
	ExpectedAnchorMatched bool                         `json:"expected_anchor_matched_preliminary_only"`
	PromptTokens          int                          `json:"prompt_tokens"`
	CompletionTokens      int                          `json:"completion_tokens"`
	UsageComplete         bool                         `json:"usage_complete"`
}

func save(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	temp := path + ".tmp"
	if err = os.WriteFile(temp, append(raw, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "evaluation stopped:", err)
		os.Exit(1)
	}
}
func run() error {
	config := flag.String("config", "config.yaml", "Existing configuration (read only)")
	corpusPath := flag.String("corpus", "evaluation/corpus-v1.json", "Ground truth corpus outside audited repositories")
	output := flag.String("output", "", "New isolated result directory")
	probe := flag.Bool("probe", false, "Safe minimal configured-provider connection check")
	onlyCase := flag.String("case", "", "Optional one-case diagnostic cohort")
	grouped := flag.Bool("grouped", false, "Use production bounded grouping and cross-group investigation handoff")
	resume := flag.Bool("resume", false, "Resume identical evaluation, retaining all completed receipts")
	timeout := flag.Int("timeout", 240, "Per-case seconds, maximum240")
	flag.Parse()
	if (!*probe && *output == "") || *timeout < 1 || *timeout > 240 {
		return fmt.Errorf("output and bounded timeout required")
	}
	if _, err := os.Stat(*config); err != nil {
		return fmt.Errorf("existing config required")
	}
	settings, err := platform.OpenSettings(*config, *config)
	if err != nil {
		return fmt.Errorf("cannot load configuration")
	}
	cfg := settings.Snapshot()
	model := cfg.ReAct.Model
	if model == "" {
		model = cfg.OpenAI.Model
	}
	if cfg.OpenAI.APIKey == "" || model == "" {
		return fmt.Errorf("configured real model required")
	}
	if *probe {
		return probeModel(context.Background(), cfg.OpenAI.URL, cfg.OpenAI.APIKey, model)
	}
	corpus, digest, err := evaluation.LoadCorpus(*corpusPath)
	if err != nil {
		return err
	}
	if err = evaluation.ValidateEvaluationLocation(corpus, *output); err != nil {
		return err
	}
	// Validate all immutable sources before creating artifacts or invoking a model.
	for _, item := range corpus.Cases {
		if item.Git == nil {
			continue
		}
		if _, _, _, e := evaluation.PrepareCase(context.Background(), "", item); e != nil {
			return e
		}
	}
	if *onlyCase != "" {
		found := false
		for _, c := range corpus.Cases {
			if c.ID == *onlyCase {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unknown diagnostic case")
		}
	}
	endpoint := sha256.Sum256([]byte(cfg.OpenAI.URL))
	revision := "unknown"
	if raw, e := exec.Command("git", "rev-parse", "HEAD").Output(); e == nil {
		revision = strings.TrimSpace(string(raw))
	}
	steps := cfg.ReAct.MaxSteps
	if steps > 16 {
		steps = 16
	}
	if steps < 2 {
		steps = 16
	}
	calls := cfg.GitAudit.MaxToolCalls
	if calls <= 0 || calls > 80 {
		calls = 80
	}
	meta := metadata{*grouped, cfg.ModelBudget.MaxTokens, cfg.VerificationModel, cfg.ModelBudget, *onlyCase, digest, revision, model, hex.EncodeToString(endpoint[:]), platform.PolicyVersion, steps, calls, float32(cfg.ReAct.Temperature), *timeout, true, false}
	if *resume {
		if err = checkResumeMetadata(filepath.Join(*output, "metadata.json"), meta); err != nil {
			return err
		}
	} else {
		if err = os.Mkdir(*output, 0700); err != nil {
			return err
		}
		if err = save(filepath.Join(*output, "metadata.json"), meta); err != nil {
			return err
		}
		if err = save(filepath.Join(*output, "ground-truth.json"), corpus); err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for i, c := range corpus.Cases {
		if *onlyCase != "" && c.ID != *onlyCase {
			continue
		}
		if ctx.Err() != nil {
			return fmt.Errorf("interrupted; partial evidence retained")
		}
		caseDir := filepath.Join(*output, c.ID)
		receiptPath := filepath.Join(caseDir, "receipt.json")
		if *resume {
			if _, e := os.Stat(receiptPath); e == nil {
				fmt.Printf("%s retained\n", c.ID)
				continue
			}
		}
		if err = os.Mkdir(caseDir, 0700); err != nil {
			return fmt.Errorf("unfinished case directory requires inspection: %s", c.ID)
		}
		repo, base, head, e := evaluation.PrepareCase(ctx, filepath.Join(caseDir, "repository"), c)
		if e != nil {
			return e
		}
		sources, contexts, e := evaluation.PrepareContextFixtures(ctx, caseDir, c)
		if e != nil {
			return e
		}
		snap := platform.Snapshot{AuditPolicy: &platform.AuditPolicy{ContextRepositories: contexts}, ProjectID: 1, SourceProjectID: 1, MRIID: i + 1, BaseSHA: base, HeadSHA: head, Title: c.ID}
		caseCtx, cancel := context.WithTimeout(ctx, time.Duration(*timeout)*time.Second)
		changes, notes, e := repo.Changes(caseCtx, snap)
		if e != nil {
			cancel()
			return fmt.Errorf("pinned fixture diff failed")
		}
		scope := platform.BuildDiff(changes, nil, 96*1024)
		scope.Notes = append(scope.Notes, notes...)
		auditor := &platform.EinoAuditor{ContextSources: sources, Repository: repo, Config: platform.AgentConfig{APIKey: cfg.OpenAI.APIKey, BaseURL: cfg.OpenAI.URL, Model: model, MaxSteps: steps, MaxTokens: cfg.ModelBudget.MaxTokens, VerificationModel: cfg.VerificationModel, MaxToolCalls: calls, Temperature: float32(cfg.ReAct.Temperature), VerifyFindings: true, GenerateDiagrams: false, Progress: func(result platform.AuditResult, trace []platform.ToolTrace) error {
			return save(filepath.Join(caseDir, "checkpoint.json"), map[string]any{"result": result, "trace": trace})
		}}}
		fmt.Printf("%s started\n", c.ID)
		started := time.Now()
		var result platform.AuditResult
		var trace []platform.ToolTrace
		var auditErr error
		if *grouped {
			plan := platform.PlanAuditGroups(changes, nil)
			plan.Notes = append(plan.Notes, notes...)
			result, trace, auditErr = auditor.AuditGroups(caseCtx, snap, plan)
		} else {
			result, trace, auditErr = auditor.Audit(caseCtx, snap, scope)
		}
		status := "completed"
		if auditErr != nil {
			status = "model_or_agent_failed"
		}
		if caseCtx.Err() != nil {
			status = "deadline_or_interruption"
		} else if auditErr == nil && len(result.CoverageNotes) > 0 {
			status = "incomplete"
		}
		usageComplete := auditErr == nil && caseCtx.Err() == nil
		cancel()
		record := receipt{ContextRepositories: contexts, Usage: platform.SummarizeModelUsage(trace, cfg.ModelBudget, usageComplete, cfg.VerificationModel != "" && cfg.VerificationModel != model), ID: c.ID, BaseSHA: base, HeadSHA: head, Status: status, ElapsedMS: time.Since(started).Milliseconds(), Result: result, Trace: trace, UsageComplete: true}
		for _, f := range result.Findings {
			if f.File == c.ExpectedAnchor.File && f.Side == c.ExpectedAnchor.Side && f.Line == c.ExpectedAnchor.Line {
				record.ExpectedAnchorMatched = true
			}
		}
		for _, tr := range trace {
			if tr.Name == "model" {
				record.PromptTokens += tr.PromptTokens
				record.CompletionTokens += tr.CompletionTokens
				if !tr.UsageReported {
					record.UsageComplete = false
				}
			}
		}
		if auditErr != nil {
			record.ErrorClass, record.HTTPStatus = classifyError(auditErr)
			record.UsageComplete = false
		}
		if record.ErrorClass == "" {
			record.ErrorClass = groupedFailureClass(result.AuditGroups)
		}
		if err = save(receiptPath, record); err != nil {
			return err
		}
		fmt.Printf("%s %s findings=%d elapsed_ms=%d tokens=%d\n", c.ID, status, len(result.Findings), record.ElapsedMS, record.PromptTokens+record.CompletionTokens)
		if record.ErrorClass != "" {
			fmt.Printf("%s error_class=%s http_status=%d\n", c.ID, record.ErrorClass, record.HTTPStatus)
		}
	}
	return nil
}

func classifyError(err error) (string, int) {
	if errors.Is(err, compose.ErrExceedMaxSteps) {
		return "agent_step_budget", 0
	}
	var response *platform.AuditResponseError
	if errors.As(err, &response) {
		return "model_response_" + response.Code, 0
	}
	if info := platform.UpstreamFailureInfo(err); info != nil {
		return info.Source + "_" + info.Kind, info.HTTPStatus
	}
	var api *modelopenai.APIError
	var request *modelopenai.RequestError
	var network net.Error
	if errors.As(err, &api) {
		return "model_api_error", api.HTTPStatusCode
	}
	if errors.As(err, &request) {
		return "model_request_error", request.HTTPStatusCode
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", 0
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline", 0
	}
	if errors.As(err, &network) {
		if network.Timeout() {
			return "network_timeout", 0
		}
		return "network_error", 0
	}
	return "agent_or_provider_error", 0
}

func checkResumeMetadata(path string, expected metadata) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var old metadata
	if json.Unmarshal(raw, &old) != nil || old != expected {
		return fmt.Errorf("resume metadata mismatch")
	}
	return nil
}
