package platform

import (
	"encoding/json"
	"sort"
	"strings"
)

func sequenceOrder(findings []Finding) []int {
	order := make([]int, len(findings))
	for i := range order {
		order[i] = i
	}
	rank := func(f Finding) int {
		n := map[string]int{"high": 0, "medium": 2, "low": 4}[f.Severity]
		if f.Confidence != "supported" {
			n++
		}
		return n
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := findings[order[i]], findings[order[j]]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		return a.ID < b.ID
	})
	return order
}

// Only pinned source facts related to this finding enter supplemental context.
func (t *auditTools) sequenceObservations(f Finding) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	links := map[string]int{}
	for _, id := range f.ObservationIDs {
		links[id] = 0
	}
	if item, ok := t.ledger[f.InvestigationID]; ok {
		for _, id := range append(append([]string{}, item.ObservationIDs...), item.CounterObservationIDs...) {
			if _, exists := links[id]; !exists {
				links[id] = 1
			}
		}
	}
	type observation struct {
		ID     string          `json:"observation_id"`
		Tool   string          `json:"tool"`
		Output json.RawMessage `json:"output"`
	}
	type candidate struct {
		priority int
		obs      observation
	}
	candidates := []candidate{}
	for _, tr := range t.trace {
		if tr.Stage == "diagram" || tr.Error != "" || !isSourceTool(tr.Name) || tr.ObservationID == "" {
			continue
		}
		var out toolOutput
		if json.Unmarshal([]byte(tr.Output), &out) != nil || !observationAtSnapshot(out, t.snap) || out.Error != "" || strings.TrimSpace(out.Text) == "" {
			continue
		}
		priority, linked := links[tr.ObservationID]
		if !linked {
			if out.RepositoryID != 0 {
				continue
			}
			var args map[string]any
			_ = json.Unmarshal([]byte(tr.Arguments), &args)
			path, _ := args["path"].(string)
			file, _ := args["file"].(string)
			if path != f.File && file != f.File {
				continue
			}
			priority = 2
		}
		candidates = append(candidates, candidate{priority, observation{tr.ObservationID, tr.Name, json.RawMessage(tr.Output)}})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].priority < candidates[j].priority })
	selected := []observation{}
	seen := map[string]bool{}
	omitted := false
	available := map[string]bool{}
	for _, candidate := range candidates {
		available[candidate.obs.ID] = true
	}
	for id := range links {
		if !available[id] {
			omitted = true
		}
	}
	encode := func() []byte {
		raw, _ := json.Marshal(struct {
			Observations []observation `json:"observations"`
			Omitted      bool          `json:"related_observations_omitted"`
		}{selected, omitted})
		return raw
	}
	for _, candidate := range candidates {
		if seen[candidate.obs.ID] {
			continue
		}
		seen[candidate.obs.ID] = true
		selected = append(selected, candidate.obs)
		if len(encode()) > 32*1024-64 {
			selected = selected[:len(selected)-1]
			omitted = true
		}
	}
	return string(encode())
}

func (t *auditTools) sequenceCheckpoint(result AuditResult) {
	// Freeze nested slices/pointers so later graph mutation cannot race with tool callbacks.
	raw, _ := json.Marshal(result)
	var frozen AuditResult
	_ = json.Unmarshal(raw, &frozen)
	t.progressMu.Lock()
	t.supplementalResult = &frozen
	t.progressMu.Unlock()
	t.checkpoint()
}
