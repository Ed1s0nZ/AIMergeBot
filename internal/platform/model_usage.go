package platform

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"math"
)

type ModelStageUsage struct {
	Stage            string `json:"stage"`
	Calls            int    `json:"calls"`
	UnknownCalls     int    `json:"unknown_calls"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}
type ModelUsage struct {
	Calls                     int               `json:"calls"`
	UnknownCalls              int               `json:"unknown_calls"`
	PromptTokens              int64             `json:"prompt_tokens"`
	CompletionTokens          int64             `json:"completion_tokens"`
	Complete                  bool              `json:"complete"`
	Stages                    []ModelStageUsage `json:"stages"`
	EstimateUnavailableReason string            `json:"estimate_unavailable_reason,omitempty"`
	EstimatedCost             *float64          `json:"estimated_cost"`
	Currency                  string            `json:"currency,omitempty"`
	MaxTokens                 int               `json:"max_tokens"`
}

func validateModelBudget(v ModelBudgetSettings) error {
	if v.MaxTokens < 0 || v.MaxTokens > 10000000 {
		return fmt.Errorf("model_budget.max_tokens must be 0–10000000")
	}
	for _, p := range []float64{v.InputPricePerMillion, v.OutputPricePerMillion, v.VerificationInputPricePerMillion, v.VerificationOutputPricePerMillion} {
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1000000 {
			return fmt.Errorf("invalid model token price")
		}
	}
	if v.Currency != "" && v.Currency != "CNY" && v.Currency != "USD" {
		return fmt.Errorf("price currency must be CNY or USD")
	}
	return nil
}

// Known sums are retained even if one request did not report usage. Never
// interpret zero calls or an interrupted attempt as a verified zero bill.
func SummarizeModelUsage(trace []ToolTrace, settings ModelBudgetSettings, terminal bool, differentVerificationModel ...bool) ModelUsage {
	out := ModelUsage{Stages: []ModelStageUsage{}, MaxTokens: settings.MaxTokens, Currency: settings.Currency}
	out.EstimateUnavailableReason = "price_not_configured"
	index := map[string]int{}
	overflow := false
	for _, tr := range trace {
		if tr.Name != "model" {
			continue
		}
		stage := tr.Stage
		if stage == "" {
			stage = "primary"
		}
		i, ok := index[stage]
		if !ok {
			i = len(out.Stages)
			index[stage] = i
			out.Stages = append(out.Stages, ModelStageUsage{Stage: stage})
		}
		out.Calls++
		out.Stages[i].Calls++
		if !tr.UsageReported || tr.Error != "" || tr.PromptTokens < 0 || tr.CompletionTokens < 0 || tr.TotalTokens < 0 || tr.PromptTokens == 0 && tr.CompletionTokens == 0 {
			out.UnknownCalls++
			out.Stages[i].UnknownCalls++
			continue
		}
		prompt, completion := int64(tr.PromptTokens), int64(tr.CompletionTokens)
		if prompt > math.MaxInt64-out.PromptTokens || completion > math.MaxInt64-out.CompletionTokens ||
			prompt > math.MaxInt64-out.Stages[i].PromptTokens || completion > math.MaxInt64-out.Stages[i].CompletionTokens {
			overflow = true
			out.UnknownCalls++
			out.Stages[i].UnknownCalls++
			continue
		}
		out.PromptTokens += int64(tr.PromptTokens)
		out.CompletionTokens += int64(tr.CompletionTokens)
		out.Stages[i].PromptTokens += int64(tr.PromptTokens)
		out.Stages[i].CompletionTokens += int64(tr.CompletionTokens)
	}
	out.Complete = terminal && out.Calls > 0 && out.UnknownCalls == 0

	if settings.Currency != "" && validateModelBudget(settings) == nil && out.Calls > out.UnknownCalls {
		cost := 0.0
		out.EstimateUnavailableReason = "verification_price_missing"
		priced := true
		different := len(differentVerificationModel) > 0 && differentVerificationModel[0]
		for _, stage := range out.Stages {
			if stage.Calls == stage.UnknownCalls {
				continue
			}
			input, output := settings.InputPricePerMillion, settings.OutputPricePerMillion
			if stage.Stage == "verification" || stage.Stage == "verification_compression" || stage.Stage == claimVerificationStage || stage.Stage == claimVerificationStage+"_compression" {
				if settings.VerificationPricingConfigured {
					input = settings.VerificationInputPricePerMillion
					output = settings.VerificationOutputPricePerMillion
				} else if different {
					priced = false
					break
				}
			}
			cost += float64(stage.PromptTokens)/1000000*input + float64(stage.CompletionTokens)/1000000*output
		}
		if priced && !math.IsInf(cost, 0) && !math.IsNaN(cost) {
			out.EstimatedCost = &cost
			out.EstimateUnavailableReason = ""
		}
	}

	if out.Calls == out.UnknownCalls {
		out.EstimateUnavailableReason = "no_reported_usage"
	}
	if overflow {
		out.EstimatedCost = nil
		out.EstimateUnavailableReason = "usage_overflow"
	}
	return out
}

const pendingModelUsage = "model request pending; token usage unknown"

func modelStartCallback(tools *auditTools, stage, model string) func(context.Context, *callbacks.RunInfo, callbacks.CallbackInput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
		if info == nil || info.Component != components.ComponentOfChatModel {
			return ctx
		}
		tools.mu.Lock()
		tools.trace = append(tools.trace, ToolTrace{Name: "model", Stage: stage, Arguments: model, Error: pendingModelUsage})
		tools.mu.Unlock()
		tools.checkpoint()
		tools.mu.Lock()
		failed := tools.progressError != "" || tools.checkpointStopped || tools.repositoryUnavailable
		tools.mu.Unlock()
		if failed {
			stopped, cancel := context.WithCancel(ctx)
			cancel()
			return stopped
		}
		return ctx
	}
}
func recordModelTrace(tools *auditTools, tr ToolTrace) {
	tools.mu.Lock()
	replaced := false
	for i := len(tools.trace) - 1; i >= 0; i-- {
		old := tools.trace[i]
		if old.Name == "model" && (old.Stage == tr.Stage || old.Stage == "primary" && tr.Stage == "") && old.Arguments == tr.Arguments && old.Error == pendingModelUsage {
			tools.trace[i] = tr
			replaced = true
			break
		}
	}
	if !replaced {
		tools.trace = append(tools.trace, tr)
	}
	tools.mu.Unlock()
	tools.checkpoint()
}
func modelFailureCallback(tools *auditTools, stage, model string) func(context.Context, *callbacks.RunInfo, error) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, _ error) context.Context {
		if info != nil && info.Component == components.ComponentOfChatModel {
			recordModelTrace(tools, ToolTrace{Name: "model", Stage: stage, Arguments: model, Error: "model request failed; token usage unavailable"})
		}
		return ctx
	}
}
