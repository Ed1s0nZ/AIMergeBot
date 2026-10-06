package platform

import (
	"math"
	"testing"
)

func TestModelUsageKnownAndUnknown(t *testing.T) {
	trace := []ToolTrace{{Name: "read_file"}, {Name: "model", UsageReported: true, PromptTokens: 100, CompletionTokens: 20}, {Name: "model", Stage: "verification", UsageReported: true, PromptTokens: 50, CompletionTokens: 10}, {Name: "model", Stage: "diagram", Error: "failed"}}
	prices := ModelBudgetSettings{Currency: "CNY", InputPricePerMillion: 2, OutputPricePerMillion: 8, MaxTokens: 1000}
	u := SummarizeModelUsage(trace, prices, true)
	if u.Calls != 3 || u.UnknownCalls != 1 || u.Complete || u.PromptTokens != 150 || u.CompletionTokens != 30 || len(u.Stages) != 3 || u.MaxTokens != 1000 {
		t.Fatalf("bad usage %+v", u)
	}
	if u.EstimatedCost == nil || math.Abs(*u.EstimatedCost-0.00054) > 1e-12 {
		t.Fatal("bad known partial estimate")
	}
	u = SummarizeModelUsage(trace[:3], prices, true)
	if !u.Complete || u.UnknownCalls != 0 {
		t.Fatal("complete reports not recognized")
	}
	u = SummarizeModelUsage(trace[:3], prices, false)
	if u.Complete {
		t.Fatal("running audit falsely complete")
	}
	u = SummarizeModelUsage(nil, prices, true)
	if u.Complete || u.EstimatedCost != nil {
		t.Fatal("empty audit fabricated zero bill")
	}
	u = SummarizeModelUsage(trace, ModelBudgetSettings{}, true)
	if u.EstimatedCost != nil || u.Currency != "" {
		t.Fatal("historical unpriced usage assigned current pricing")
	}
	for _, tr := range []ToolTrace{{Name: "model"}, {Name: "model", UsageReported: true}, {Name: "model", UsageReported: true, PromptTokens: -1, CompletionTokens: 10}} {
		u = SummarizeModelUsage([]ToolTrace{tr}, prices, true)
		if u.Complete || u.UnknownCalls != 1 || u.EstimatedCost != nil {
			t.Fatal("invalid usage accepted")
		}
	}
}
func TestModelBudgetPriceValidation(t *testing.T) {
	for _, v := range []float64{-1, 1000001, math.NaN(), math.Inf(1)} {
		if validateModelBudget(ModelBudgetSettings{InputPricePerMillion: v}) == nil {
			t.Fatal("invalid price accepted")
		}
	}
	if validateModelBudget(ModelBudgetSettings{Currency: "unknown"}) == nil {
		t.Fatal("currency accepted")
	}
	for _, currency := range []string{"", "USD", "CNY"} {
		if validateModelBudget(ModelBudgetSettings{Currency: currency}) != nil {
			t.Fatal(currency)
		}
	}
}

func TestDifferentVerifierPricesNeverUsePrimaryPrice(t *testing.T) {
	trace := []ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 1000000, CompletionTokens: 1000000}, {Name: "model", Stage: "verification", UsageReported: true, PromptTokens: 1000000, CompletionTokens: 1000000}}
	prices := ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2, OutputPricePerMillion: 8}
	unknown := SummarizeModelUsage(trace, prices, true, true)
	if !unknown.Complete || unknown.EstimatedCost != nil || unknown.EstimateUnavailableReason != "verification_price_missing" {
		t.Fatal("different verifier assigned primary price")
	}
	prices.VerificationPricingConfigured = true
	prices.VerificationInputPricePerMillion = 1
	prices.VerificationOutputPricePerMillion = 3
	known := SummarizeModelUsage(trace, prices, true, true)
	if known.EstimatedCost == nil || *known.EstimatedCost != 14 {
		t.Fatal("separate prices mixed")
	}
	same := SummarizeModelUsage(trace, ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2, OutputPricePerMillion: 8}, true, false)
	if same.EstimatedCost == nil || *same.EstimatedCost != 20 {
		t.Fatal("same model compatibility lost")
	}
}

func TestInvestigationReviewAndCompressionUseVerificationPrices(t *testing.T) {
	for _, stage := range []string{claimVerificationStage, claimVerificationStage + "_compression"} {
		trace := []ToolTrace{{Name: "model", Stage: stage, UsageReported: true, PromptTokens: 1000000, CompletionTokens: 1000000}}
		prices := ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2, OutputPricePerMillion: 8}
		unknown := SummarizeModelUsage(trace, prices, true, true)
		if unknown.EstimatedCost != nil || unknown.EstimateUnavailableReason != "verification_price_missing" {
			t.Fatal(stage, unknown)
		}
		prices.VerificationPricingConfigured = true
		prices.VerificationInputPricePerMillion = 1
		prices.VerificationOutputPricePerMillion = 3
		known := SummarizeModelUsage(trace, prices, true, true)
		if known.EstimatedCost == nil || *known.EstimatedCost != 4 {
			t.Fatal(stage, known)
		}
	}
}

func TestModelUsageMalformedTotalAndOverflow(t *testing.T) {
	prices := ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 1, OutputPricePerMillion: 1}
	invalid := SummarizeModelUsage([]ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 1, CompletionTokens: 1, TotalTokens: -1}}, prices, true)
	if invalid.Complete || invalid.UnknownCalls != 1 || invalid.EstimatedCost != nil {
		t.Fatalf("negative total accepted: %+v", invalid)
	}
	for _, stage := range []string{"primary", "verification"} {
		for _, completion := range []bool{false, true} {
			first := ToolTrace{Name: "model", Stage: "primary", UsageReported: true, PromptTokens: math.MaxInt64, CompletionTokens: 1}
			second := ToolTrace{Name: "model", Stage: stage, UsageReported: true, PromptTokens: 1, CompletionTokens: 1}
			if completion {
				first.PromptTokens, first.CompletionTokens = 1, math.MaxInt64
			}
			u := SummarizeModelUsage([]ToolTrace{first, second}, prices, true)
			if u.Complete || u.UnknownCalls != 1 || u.EstimatedCost != nil || u.EstimateUnavailableReason != "usage_overflow" || u.PromptTokens != int64(first.PromptTokens) || u.CompletionTokens != int64(first.CompletionTokens) {
				t.Fatalf("overflow not isolated for stage %s/completion %v: %+v", stage, completion, u)
			}
		}
	}
}
