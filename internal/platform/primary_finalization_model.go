package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	em "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type primaryRecordingState struct {
	mu          sync.Mutex
	used, limit int
	asked       bool
}

type primaryRecordingModel struct {
	em.ToolCallingChatModel
	state   *primaryRecordingState
	tools   *auditTools
	prompt  string
	rewrite func(context.Context, []*schema.Message) []*schema.Message
}

func newPrimaryRecordingModel(m em.ToolCallingChatModel, t *auditTools, limit int, prompt string, rewrite ...func(context.Context, []*schema.Message) []*schema.Message) *primaryRecordingModel {
	model := &primaryRecordingModel{ToolCallingChatModel: m, state: &primaryRecordingState{limit: limit}, tools: t, prompt: prompt}
	if len(rewrite) > 0 {
		model.rewrite = rewrite[0]
	}
	return model
}
func (m *primaryRecordingModel) IsCallbacksEnabled() bool {
	if c, ok := m.ToolCallingChatModel.(interface{ IsCallbacksEnabled() bool }); ok {
		return c.IsCallbacksEnabled()
	}
	return false
}
func (m *primaryRecordingModel) GetType() string {
	if c, ok := m.ToolCallingChatModel.(interface{ GetType() string }); ok {
		return c.GetType()
	}
	return "PrimaryRecordingModel"
}
func (m *primaryRecordingModel) WithTools(infos []*schema.ToolInfo) (em.ToolCallingChatModel, error) {
	bound, err := m.ToolCallingChatModel.WithTools(infos)
	if err != nil {
		return nil, err
	}
	if bound == nil {
		return nil, errors.New("primary model tool binding unavailable")
	}
	return &primaryRecordingModel{ToolCallingChatModel: bound, state: m.state, tools: m.tools, prompt: m.prompt, rewrite: m.rewrite}, nil
}
func (m *primaryRecordingModel) Stream(context.Context, []*schema.Message, ...em.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("streaming unavailable with primary decision budget")
}
func (m *primaryRecordingModel) decision(ctx context.Context, input []*schema.Message, opts ...em.Option) (*schema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.state.mu.Lock()
	if m.state.used >= m.state.limit {
		m.state.mu.Unlock()
		return nil, compose.ErrExceedMaxSteps
	}
	m.state.used++
	current, limit, recordingPass := m.state.used, m.state.limit, m.state.asked
	m.state.mu.Unlock()
	prompt := m.prompt
	if recordingPass {
		prompt += "\n" + primaryRecordingFinalizationRequest
	}
	messages := primaryDecisionMessages(input, prompt, current, limit, m.tools.primaryProgressNavigation)
	msg, err := m.ToolCallingChatModel.Generate(ctx, messages, opts...)
	if err == nil && msg != nil && current >= limit && len(msg.ToolCalls) > 0 {
		return nil, compose.ErrExceedMaxSteps
	}
	return msg, err
}
func (m *primaryRecordingModel) Generate(ctx context.Context, input []*schema.Message, opts ...em.Option) (*schema.Message, error) {
	msg, err := m.decision(ctx, input, opts...)
	if err != nil || msg == nil || len(msg.ToolCalls) > 0 {
		return msg, err
	}
	draft, parseErr := ParseResult(msg.Content)
	if parseErr != nil || ctx.Err() != nil || !m.tools.primaryRecordingFinalizationRequired() {
		return msg, nil
	}
	m.state.mu.Lock()
	retry := !m.state.asked && m.state.limit-m.state.used >= 2
	if retry {
		m.state.asked = true
	}
	m.state.mu.Unlock()
	if !retry {
		return msg, nil
	}
	// A valid early final could contain a legal proposal never submitted as a
	// tool. Retain it through the original validator before another request can
	// fail or be cancelled; unvalidated proposals and model ledger are ignored.
	for _, f := range draft.Findings {
		_, _ = m.tools.acceptFinding(ctx, f)
	}
	follow := append([]*schema.Message(nil), input...)
	follow = append(follow, schema.AssistantMessage(msg.Content, nil), schema.UserMessage(primaryRecordingFinalizationRequest))
	if m.rewrite != nil {
		follow = m.rewrite(ctx, follow)
	}
	return m.decision(ctx, follow, opts...)
}

const primaryRecordingFinalizationRequest = `The server found unfinished structured investigation recording after this draft final JSON. Use the remaining ORIGINAL decision/tool/time budget to make a deliberate recording pass before final JSON. Reuse successful primary BASE/HEAD IDs and explicitly set before_observation_ids/after_observation_ids using record_pr_context; do not repeat the whole plan just to link sources. Record actual caller/transform/contract relationships when supported. For an inferred related-repository connection, inspect relevant available configuration, deployment or routing observations before claiming no connection evidence exists; a repository that has been partially read can still have unread contract sources. Preserve existing plan identities, actual missing evidence, unknown/inferred edges and failed history. Do not invent source IDs, convert names into cited links, certify safety or execute code. Explicitly resubmit a finding if its saved context must change. A supported claim needs no finding. If the necessary evidence remains unavailable, return the final JSON with its actual limitations; the server does not require every investigation to be completed.`

// This is a finite structural gate, not a semantic verdict or permission to
// create facts. A partial/unknown record can still finish after the one request.
func (t *auditTools) primaryRecordingFinalizationRequired() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	max := t.maxCalls
	if max == 0 {
		max = 80
	}
	if len(t.scope.Added)+len(t.scope.Removed)+len(t.scope.Metadata) == 0 || t.calls >= max || t.repositoryUnavailable || t.checkpointStopped || t.progressError != "" {
		return false
	}
	hasSource := false
	for _, tr := range t.trace {
		var out toolOutput
		if (tr.Stage == "" || tr.Stage == "primary") && tr.Error == "" && isSourceTool(tr.Name) && json.Unmarshal([]byte(tr.Output), &out) == nil && out.EvidenceEligible && out.Error == "" && out.ObservationID == tr.ObservationID && strings.TrimSpace(out.Text) != "" && observationAtSnapshot(out, t.snap) {
			hasSource = true
			break
		}
	}
	if !hasSource {
		return false
	}
	if len(t.ledger) == 0 {
		return true
	}
	metadata := t.primaryMetadataOnlySourcesLocked()
	for _, item := range t.ledger {
		if len(investigationRecordingGapsForBasis(item, metadataOnlyInvestigation(item, metadata))) > 0 {
			return true
		}
	}
	return false
}
