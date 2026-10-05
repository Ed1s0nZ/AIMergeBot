package platform

import (
	"context"
	"errors"
	"sync"

	em "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var ErrModelTokenBudget = errors.New("model token stopping threshold reached")
var ErrModelUsageUnknown = errors.New("model usage unavailable; further budgeted calls stopped")

type ModelBudgetSettings struct {
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
}
type modelBudgetKey struct{}
type modelTokenBudget struct {
	mu          sync.Mutex
	limit, used int
	unknown     bool
}

// One budget per audit, inherited by groups and all supplemental stages.
func withModelBudget(ctx context.Context, limit int) context.Context {
	if ctx.Value(modelBudgetKey{}) != nil || limit <= 0 {
		return ctx
	}
	return context.WithValue(ctx, modelBudgetKey{}, &modelTokenBudget{limit: limit})
}

type budgetedModel struct{ em.ToolCallingChatModel }

// Preserve SDK callback ownership; otherwise Eino would emit a second model
// callback around the wrapper and inflate persisted usage statistics.
func (m *budgetedModel) IsCallbacksEnabled() bool {
	if c, ok := m.ToolCallingChatModel.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return false
}
func (m *budgetedModel) GetType() string {
	if c, ok := m.ToolCallingChatModel.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "BudgetedModel"
}
func budgetModel(m em.ToolCallingChatModel) em.ToolCallingChatModel { return &budgetedModel{m} }
func (m *budgetedModel) WithTools(t []*schema.ToolInfo) (em.ToolCallingChatModel, error) {
	bound, err := m.ToolCallingChatModel.WithTools(t)
	if err != nil {
		return nil, err
	}
	return budgetModel(bound), nil
}
func (m *budgetedModel) Generate(ctx context.Context, input []*schema.Message, opts ...em.Option) (*schema.Message, error) {
	b, _ := ctx.Value(modelBudgetKey{}).(*modelTokenBudget)
	if b == nil {
		return m.ToolCallingChatModel.Generate(ctx, input, opts...)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.unknown {
		return nil, ErrModelUsageUnknown
	}
	if b.used >= b.limit {
		return nil, ErrModelTokenBudget
	}
	out, err := m.ToolCallingChatModel.Generate(ctx, input, opts...)
	if err != nil || out == nil || out.ResponseMeta == nil || out.ResponseMeta.Usage == nil {
		b.unknown = true
		return out, err
	}
	u := out.ResponseMeta.Usage
	if u.PromptTokens < 0 || u.CompletionTokens < 0 || u.TotalTokens < 0 {
		b.unknown = true
		return out, err
	}
	n := int64(u.PromptTokens) + int64(u.CompletionTokens)
	if int64(u.TotalTokens) > n {
		n = int64(u.TotalTokens)
	}
	if n <= 0 {
		b.unknown = true
		return out, err
	}
	// Only threshold state is needed; saturate to avoid arithmetic overflow.
	if n >= int64(b.limit-b.used) {
		b.used = b.limit
	} else {
		b.used += int(n)
	}
	return out, err
}
func (m *budgetedModel) Stream(ctx context.Context, input []*schema.Message, opts ...em.Option) (*schema.StreamReader[*schema.Message], error) {
	if ctx.Value(modelBudgetKey{}) != nil {
		return nil, errors.New("streaming unavailable with model token budget")
	}
	return m.ToolCallingChatModel.Stream(ctx, input, opts...)
}
