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
	Calls            int               `json:"calls"`
	UnknownCalls     int               `json:"unknown_calls"`
	PromptTokens     int64             `json:"prompt_tokens"`
	CompletionTokens int64             `json:"completion_tokens"`
	Complete         bool              `json:"complete"`
	Stages           []ModelStageUsage `json:"stages"`
	EstimatedCost    *float64          `json:"estimated_cost"`
	Currency         string            `json:"currency,omitempty"`
	MaxTokens        int               `json:"max_tokens"`
}

func validateModelBudget(v ModelBudgetSettings) error {
	if v.MaxTokens < 0 || v.MaxTokens > 10000000 {
		return fmt.Errorf("model_budget.max_tokens must be 0–10000000")
	}
	for _, p := range []float64{v.InputPricePerMillion, v.OutputPricePerMillion} {
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
func SummarizeModelUsage(trace []ToolTrace, settings ModelBudgetSettings, terminal bool) ModelUsage {
	out := ModelUsage{Stages: []ModelStageUsage{}, MaxTokens: settings.MaxTokens, Currency: settings.Currency}
	index := map[string]int{}
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
		if !tr.UsageReported || tr.Error != "" || tr.PromptTokens < 0 || tr.CompletionTokens < 0 || tr.PromptTokens == 0 && tr.CompletionTokens == 0 {
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
		cost := float64(out.PromptTokens)/1000000*settings.InputPricePerMillion + float64(out.CompletionTokens)/1000000*settings.OutputPricePerMillion
		if !math.IsInf(cost, 0) && !math.IsNaN(cost) {
			out.EstimatedCost = &cost
		}
	}
	return out
}
func modelFailureCallback(tools *auditTools, stage, model string) func(context.Context, *callbacks.RunInfo, error) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, _ error) context.Context {
		if info != nil && info.Component == components.ComponentOfChatModel {
			tools.mu.Lock()
			tools.trace = append(tools.trace, ToolTrace{Name: "model", Stage: stage, Arguments: model, Error: "model request failed; token usage unavailable"})
			tools.mu.Unlock()
			tools.checkpoint()
		}
		return ctx
	}
}
