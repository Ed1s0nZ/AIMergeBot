package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type readArgs struct {
	Path  string `json:"path" jsonschema:"description=Repository relative file path"`
	Base  bool   `json:"base" jsonschema:"description=Read base snapshot instead of head"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}
type listArgs struct {
	Page int `json:"page"`
}
type searchArgs struct {
	Query      string `json:"query"`
	Page       int    `json:"page"`
	Cursor     int    `json:"cursor"`
	Path       string `json:"path"`
	Extension  string `json:"extension"`
	Regex      bool   `json:"regex"`
	IgnoreCase bool   `json:"ignore_case"`
	Base       bool   `json:"base"`
}
type toolOutput struct {
	EvidenceEligible       bool               `json:"evidence_eligible"`
	EligibleObservationIDs []string           `json:"eligible_observation_ids,omitempty"`
	Metadata               *GitChangeMetadata `json:"metadata,omitempty"`
	ObservationID          string             `json:"observation_id"`
	BaseSHA                string             `json:"base_sha"`
	HeadSHA                string             `json:"head_sha"`
	NextCursor             int                `json:"next_cursor,omitempty"`
	Remaining              bool               `json:"remaining,omitempty"`
	Text                   string             `json:"text"`
	Files                  []string           `json:"files,omitempty"`
	More                   bool               `json:"more,omitempty"`
	Error                  string             `json:"error,omitempty"`
}

type auditTools struct {
	stage              string
	observationPrefix  string
	pages              map[string]*paginationCoverage
	progress           func(AuditResult, []ToolTrace) error
	progressMu         sync.Mutex
	supplementalResult *AuditResult
	progressError      string
	repo               Repository
	snap               Snapshot
	mu                 sync.Mutex
	cache              map[string]string
	trace              []ToolTrace
	calls              int
	cacheBytes         int
	maxCalls           int
	scope              DiffScope
	findings           map[string]Finding
	ledger             map[string]Investigation
	pending            map[string]ToolTrace
}

func (t *auditTools) read(ctx context.Context, p string, base bool) (string, error) {
	key := fmt.Sprintf("%t:%s", base, p)
	t.mu.Lock()
	cached, ok := t.cache[key]
	t.mu.Unlock()
	if ok {
		return cached, nil
	}
	text, err := t.repo.ReadFile(ctx, t.snap, p, base)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	if cached, ok := t.cache[key]; ok {
		t.mu.Unlock()
		return cached, nil
	}
	if t.cacheBytes+len(text) > 16*1024*1024 {
		t.mu.Unlock()
		return "", fmt.Errorf("repository cache byte budget exhausted")
	}
	t.cache[key] = text
	t.cacheBytes += len(text)
	t.mu.Unlock()
	return text, nil
}
func (t *auditTools) invoke(name string, args any, fn func() (toolOutput, error)) (toolOutput, error) {
	start := time.Now()
	t.mu.Lock()
	t.calls++
	callID := t.calls
	limit := t.maxCalls
	if limit == 0 {
		limit = 80
	}
	over := t.calls > limit
	t.mu.Unlock()
	var out toolOutput
	var err error
	if over {
		err = fmt.Errorf("tool call budget exhausted")
	} else {
		out, err = fn()
	}
	prefix := t.observationPrefix
	if prefix == "" {
		prefix = "observation"
	}
	out.ObservationID = fmt.Sprintf("%s-%d", prefix, callID)
	out.BaseSHA = t.snap.BaseSHA
	out.HeadSHA = t.snap.HeadSHA
	raw, _ := json.Marshal(args)
	trace := ToolTrace{Name: name, Stage: t.stage, Arguments: string(raw), DurationMS: time.Since(start).Milliseconds(), Partial: out.More}
	if err != nil {
		trace.Error = err.Error()
		out.Error = err.Error()
	}
	out.EvidenceEligible = err == nil && isSourceTool(name) && strings.TrimSpace(out.Text) != ""
	t.mu.Lock()
	if err != nil {
		for i := len(t.trace) - 1; i >= 0 && len(out.EligibleObservationIDs) < 20; i-- {
			prior := t.trace[i]
			if prior.Error != "" || !isSourceTool(prior.Name) {
				continue
			}
			var evidence toolOutput
			if json.Unmarshal([]byte(prior.Output), &evidence) == nil && evidence.Error == "" && strings.TrimSpace(evidence.Text) != "" && evidence.BaseSHA == t.snap.BaseSHA && evidence.HeadSHA == t.snap.HeadSHA {
				out.EligibleObservationIDs = append(out.EligibleObservationIDs, prior.ObservationID)
			}
		}
	}
	encoded, _ := json.Marshal(out)
	trace.Output = string(encoded)
	trace.ObservationID = out.ObservationID
	if t.pending == nil {
		t.pending = map[string]ToolTrace{}
	}
	key := queryKey(name, args)
	if complete, tracked := t.paginationComplete(name, key, args, out, err != nil); tracked {
		trace.Partial = !complete
	}
	if trace.Partial || trace.Error != "" {
		t.pending[key] = trace
	} else {
		delete(t.pending, key)
	}
	t.trace = append(t.trace, trace)
	t.mu.Unlock()
	t.checkpoint()
	// Tool failures are visible to the model, but retained to mark the run incomplete.
	return out, nil
}
func (t *auditTools) file(ctx context.Context, a readArgs) (toolOutput, error) {
	return t.invoke("read_file", a, func() (toolOutput, error) {
		text, err := t.read(ctx, a.Path, a.Base)
		if err != nil {
			return toolOutput{}, err
		}
		lines := strings.Split(text, "\n")
		start, end := a.Start, a.End
		truncated := a.End > 0 && a.End-a.Start > 199
		if start == 0 {
			start = 1
		}
		if end == 0 {
			end = start + 199
		}
		if start < 1 || end < start || start > len(lines) {
			return toolOutput{}, fmt.Errorf("invalid line range")
		}
		if end > len(lines) {
			end = len(lines)
		}
		if end-start > 199 {
			end = start + 199
		}
		var b strings.Builder
		for n := start; n <= end; n++ {
			line := fmt.Sprintf("%d: %s\n", n, lines[n-1])
			if b.Len()+len(line) > 16000 {
				return toolOutput{Text: b.String(), More: true}, nil
			}
			b.WriteString(line)
		}
		return toolOutput{Text: b.String(), More: truncated, Remaining: end < len(lines)}, nil
	})
}
func (t *auditTools) list(ctx context.Context, a listArgs) (toolOutput, error) {
	return t.invoke("list_files", a, func() (toolOutput, error) {
		if a.Page == 0 {
			a.Page = 1
		}
		files, more, err := t.repo.ListFiles(ctx, t.snap, a.Page)
		bytes := 0
		bounded := []string{}
		for _, p := range files {
			if bytes+len(p) > 16000 {
				return toolOutput{Files: bounded, More: true}, fmt.Errorf("file listing byte budget exceeded; use list_directory for bounded exploration")
			}
			bounded = append(bounded, p)
			bytes += len(p)
		}
		return toolOutput{Files: bounded, More: more}, err
	})
}
