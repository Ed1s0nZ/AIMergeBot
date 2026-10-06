package platform

import (
	"context"
	"encoding/json"
	"fmt"
)

// The inventory is an ordinary budgeted list_files observation, not source.
// Keep paths in the untrusted user message and persist before the first model.
func (t *auditTools) primaryRepositoryNavigation(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	out, err := t.list(ctx, listArgs{Page: 1})
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	t.mu.Lock()
	unavailable, stopped, failedCheckpoint := t.repositoryUnavailable, t.checkpointStopped, t.progressError != ""
	t.mu.Unlock()
	if unavailable {
		return "", fmt.Errorf("%w: primary HEAD inventory preflight", ErrRepositoryUnavailable)
	}
	if stopped {
		return "", ErrConflict
	}
	if failedCheckpoint {
		return "", fmt.Errorf("primary HEAD inventory checkpoint unavailable")
	}
	// Retain the original failed trace, but don't repeat raw provider errors in
	// the initial message. The model can retry using the existing tool contract.
	if out.Error != "" {
		out.Error = "Primary HEAD file inventory unavailable; do not infer repository size or absence of callers. Use bounded repository tools or preserve the actual limitation."
	}
	raw, _ := json.Marshal(out)
	return "\nUntrusted primary HEAD file inventory (navigation only, not source evidence; this is not the PR changed-path list; more=true requires subsequent pages or an explicit limitation):\n" + string(raw), nil
}

// Caller holds t.mu. Count filenames only; neither enumeration nor a source
// read establishes a call relationship. No paths enter the system projection.
func (t *auditTools) primaryHeadInventoryLocked() (string, int) {
	type inventoryPage struct {
		files []string
		more  bool
	}
	pages := map[int]inventoryPage{}
	failed := false
	for _, tr := range t.trace {
		if tr.Name != "list_files" || (tr.Stage != "" && tr.Stage != "primary") {
			continue
		}
		var out toolOutput
		var args listArgs
		if json.Unmarshal([]byte(tr.Output), &out) != nil || json.Unmarshal([]byte(tr.Arguments), &args) != nil || out.ObservationID != tr.ObservationID || out.RepositoryID != 0 || out.BaseSHA != t.snap.BaseSHA || out.HeadSHA != t.snap.HeadSHA || out.EvidenceEligible {
			continue
		}
		if args.Page == 0 {
			args.Page = 1
		}
		if args.Page < 1 {
			continue
		}
		if tr.Error != "" || out.Error != "" {
			failed = true
			continue
		}
		pages[args.Page] = inventoryPage{out.Files, out.More}
	}
	if len(pages) == 0 {
		if failed {
			return "unavailable", 0
		}
		return "not_inspected", 0
	}
	last := 0
	for n, page := range pages {
		if !page.more && (last == 0 || n < last) {
			last = n
		}
	}
	seenFiles := map[string]bool{}
	seenPages := 0
	for n, page := range pages {
		if last > 0 && n > last {
			continue
		}
		seenPages++
		for _, file := range page.files {
			seenFiles[file] = true
		}
	}
	// Comparing unique positive page counts avoids looping over a model-chosen
	// page number, which may be extremely large.
	if last > 0 && seenPages == last {
		return "complete", len(seenFiles)
	}
	return "partial", len(seenFiles)
}
