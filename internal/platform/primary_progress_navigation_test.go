package platform

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/schema"
	"strings"
	"testing"
)

func TestProgressNavigationUsesOnlyRuntimeMetadata(t *testing.T) {
	root, snap, f, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	ctx := context.Background()
	empty := tools.primaryProgressNavigation()
	if !strings.Contains(empty, `"uninspected_context_ids":[2]`) || !strings.Contains(empty, `"ledger_count":0`) {
		t.Fatal("missing initial state", empty)
	}
	head, _ := tools.file(ctx, readArgs{Path: f.File, Start: 1, End: 3})
	base, _ := tools.file(ctx, readArgs{Path: f.File, Base: true, Start: 1, End: 3})
	tools.ledger = map[string]Investigation{"secret": {Claim: "PRIVATE MODEL CLAIM", NextSteps: []string{"PRIVATE NEXT STEP"}}}
	tools.mu.Lock()
	var out toolOutput
	json.Unmarshal([]byte(tools.trace[0].Output), &out)
	out.Text = "PRIVATE SOURCE TEXT"
	raw, _ := json.Marshal(out)
	tools.trace[0].Output = string(raw)
	tools.mu.Unlock()
	nav := tools.primaryProgressNavigation()
	if strings.Contains(nav, "PRIVATE") || strings.Contains(nav, f.File) {
		t.Fatal("untrusted content entered system navigation", nav)
	}
	if !strings.Contains(nav, `"recent_primary_base_source_ids":["`+base.ObservationID+`"]`) || !strings.Contains(nav, `"recent_primary_head_source_ids":["`+head.ObservationID+`"]`) {
		t.Fatal("snapshot side navigation lost", nav)
	}
	related, _ := tools.contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: f.File, Start: 1, End: 3})
	if related.Error != "" {
		t.Fatal(related.Error)
	}
	nav = tools.primaryProgressNavigation()
	if strings.Contains(nav, "uninspected_context_ids") || strings.Contains(nav, related.ObservationID) {
		t.Fatal("context inspection not recognized or mixed with primary sides", nav)
	}
	tools.mu.Lock()
	latest := len(tools.trace) - 1
	var stale toolOutput
	json.Unmarshal([]byte(tools.trace[latest].Output), &stale)
	stale.HeadSHA = strings.Repeat("0", 40)
	staleRaw, _ := json.Marshal(stale)
	tools.trace[latest].Output = string(staleRaw)
	tools.trace[1].Stage = "verification"
	tools.mu.Unlock()
	nav = tools.primaryProgressNavigation()
	if !strings.Contains(nav, `"uninspected_context_ids":[2]`) || strings.Contains(nav, base.ObservationID) {
		t.Fatal("stale context or independent verification retroactively covered primary", nav)
	}
}
func TestProgressNavigationReplacementDoesNotAccumulate(t *testing.T) {
	n := 0
	rewrite := primaryRoundRewriter(4, "trusted", func(_ context.Context, m []*schema.Message) []*schema.Message { return m }, func() string { n++; return " nav revision " + strings.Repeat("x", n) })
	original := []*schema.Message{schema.SystemMessage("trusted"), schema.UserMessage("source")}
	m := original
	for i := 0; i < 3; i++ {
		m = rewrite(context.Background(), m)
	}
	if strings.Count(m[0].Content, "nav revision") != 1 || !strings.Contains(m[0].Content, "nav revision xxx") || original[0].Content != "trusted" {
		t.Fatal("navigation accumulated or mutated original", m[0].Content)
	}
}
func TestPrimarySourceSidesRejectMixedBatch(t *testing.T) {
	tr := ToolTrace{Name: "read_files", Arguments: `{"files":[{"path":"x","base":true},{"path":"y","base":false}]}`}
	base, head := primarySourceSides(tr)
	if base || head {
		t.Fatal("mixed batch used for one snapshot side")
	}
	tr.Arguments = `{"files":[{"path":"x","base":true}]}`
	base, head = primarySourceSides(tr)
	if !base || head {
		t.Fatal("base-only batch unrecognized")
	}
}
