package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type gitArgs struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path"`
	Base    bool   `json:"base"`
	Cursor  int    `json:"cursor"`
	Skip    int    `json:"skip"`
	Limit   int    `json:"limit"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Query   string `json:"query"`
}

func (t *auditTools) gitQuery(ctx context.Context, name string, a gitArgs) (toolOutput, error) {
	return t.invoke(name, a, func() (toolOutput, error) {
		g, ok := t.repo.(*GitRepository)
		if !ok {
			return toolOutput{}, fmt.Errorf("this tool requires native Git; enable git_audit")
		}
		if !validScope(a.Path) || !validScope(a.OldPath) {
			return toolOutput{}, fmt.Errorf("invalid path")
		}
		ref, e := g.ref(t.snap, a.Base)
		if e != nil {
			return toolOutput{}, e
		}
		if a.Skip < 0 || a.Skip > 10000 {
			return toolOutput{}, fmt.Errorf("skip must be 0–10000")
		}
		limit := a.Limit
		if limit == 0 {
			limit = 20
		}
		if limit < 1 || limit > 100 {
			return toolOutput{}, fmt.Errorf("limit must be 1–100")
		}
		var args []string
		switch name {
		case "get_diff":
			args = []string{"diff", "--no-ext-diff", "--no-textconv", "--unified=10", t.snap.BaseSHA, t.snap.HeadSHA, "--"}
			if a.Path != "" {
				args = append(args, a.Path)
			}
		case "compare_files":
			if !validPath(a.Path) {
				return toolOutput{}, fmt.Errorf("path required")
			}
			old := a.OldPath
			if old == "" {
				old = a.Path
			}
			args = []string{"diff", "--no-ext-diff", "--no-textconv", "--unified=10", t.snap.BaseSHA + ":" + old, t.snap.HeadSHA + ":" + a.Path}
		case "get_history", "search_history":
			args = []string{"log", "--no-ext-diff", "--no-textconv", "--format=commit %H%nDate: %aI%nSubject: %s", "--max-count=" + strconv.Itoa(limit), "--skip=" + strconv.Itoa(a.Skip), "-p"}
			if name == "search_history" {
				if len(a.Query) < 1 || len(a.Query) > 500 {
					return toolOutput{}, fmt.Errorf("query must have 1–500 bytes")
				}
				args = append(args, "-S"+a.Query)
			}
			args = append(args, ref, "--")
			if a.Path != "" {
				args = append(args, a.Path)
			}
		case "git_blame":
			if !validPath(a.Path) || a.Start < 1 || a.End < a.Start || a.End-a.Start > 199 {
				return toolOutput{}, fmt.Errorf("blame requires path and range of at most 200 lines")
			}
			args = []string{"blame", "--line-porcelain", "-L", fmt.Sprintf("%d,%d", a.Start, a.End), ref, "--", a.Path}
		default:
			return toolOutput{}, fmt.Errorf("unknown Git query")
		}
		raw, e := g.command(ctx, args...)
		if e != nil {
			return toolOutput{}, e
		}
		if name == "git_blame" || name == "get_history" || name == "search_history" {
			raw = "History bounded to " + strconv.Itoa(limit) + " commits; shallow=" + strconv.FormatBool(g.HistoryLimited) + "\n" + raw
		}
		return textPage(strings.TrimSpace(raw), a.Cursor)
	})
}
func (t *auditTools) diff(ctx context.Context, a gitArgs) (toolOutput, error) {
	return t.gitQuery(ctx, "get_diff", a)
}
func (t *auditTools) compare(ctx context.Context, a gitArgs) (toolOutput, error) {
	return t.gitQuery(ctx, "compare_files", a)
}
func (t *auditTools) history(ctx context.Context, a gitArgs) (toolOutput, error) {
	return t.gitQuery(ctx, "get_history", a)
}
func (t *auditTools) blame(ctx context.Context, a gitArgs) (toolOutput, error) {
	return t.gitQuery(ctx, "git_blame", a)
}
func (t *auditTools) historySearch(ctx context.Context, a gitArgs) (toolOutput, error) {
	return t.gitQuery(ctx, "search_history", a)
}
