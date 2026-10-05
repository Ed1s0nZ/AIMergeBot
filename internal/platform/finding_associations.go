package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const associationMaxItems = 50

type FindingAssociationDecision struct {
	Revision  int64  `json:"revision"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	Actor     int64  `json:"actor"`
	CreatedAt string `json:"created_at"`
}
type FindingAssociation struct {
	ID               string                       `json:"id"`
	FindingID        string                       `json:"finding_id"`
	PriorRunID       int64                        `json:"prior_run_id"`
	PriorFindingID   string                       `json:"prior_finding_id"`
	OldPath          string                       `json:"old_path"`
	NewPath          string                       `json:"new_path"`
	PriorHeadSHA     string                       `json:"prior_head_sha"`
	HeadSHA          string                       `json:"head_sha"`
	RenameBaseSHA    string                       `json:"rename_base_sha"`
	Metadata         GitChangeMetadata            `json:"metadata"`
	Evidence         string                       `json:"evidence"`
	Trigger          string                       `json:"trigger"`
	RiskType         string                       `json:"risk_type"`
	Ambiguous        bool                         `json:"ambiguous"`
	State            FindingAssociationDecision   `json:"state"`
	History          []FindingAssociationDecision `json:"history"`
	HistoryTruncated bool                         `json:"history_truncated"`
}
type FindingAssociations struct {
	Items       []FindingAssociation `json:"items"`
	Truncated   bool                 `json:"truncated"`
	Limitations []string             `json:"limitations"`
}

func associationTerminal(status string) bool {
	switch status {
	case "succeeded", "incomplete", "failed", "cancelled", "skipped", "interrupted":
		return true
	}
	return false
}
func associationScope(a, b Run) bool {
	source := func(s Snapshot) int {
		if s.SourceProjectID == 0 {
			return s.ProjectID
		}
		return s.SourceProjectID
	}
	ac, _ := json.Marshal(append([]ContextRepository{}, contextPolicyItems(a.Snapshot)...))
	bc, _ := json.Marshal(append([]ContextRepository{}, contextPolicyItems(b.Snapshot)...))
	return a.ProjectID > 0 && a.ProjectID == b.ProjectID && a.MRIID > 0 && a.MRIID == b.MRIID && source(a.Snapshot) == source(b.Snapshot) && string(ac) == string(bc) && a.ID > b.ID && b.ID > 0 && associationTerminal(a.Status) && associationTerminal(b.Status) && commitID.MatchString(a.BaseSHA) && commitID.MatchString(a.HeadSHA) && commitID.MatchString(b.BaseSHA) && commitID.MatchString(b.HeadSHA)
}
func associationLine(s Snapshot, f Finding) bool {
	return (f.Side == "" || f.Side == "head") && (f.AnchorType == "" || f.AnchorType == "line") && f.Line > 0 && f.ID != "" && f.Fingerprint != "" && f.Fingerprint == findingFingerprint(s, f)
}
func associationRename(m GitChangeMetadata) bool {
	regular := func(e *GitEntry) bool {
		return e != nil && e.Type == "blob" && (e.Mode == "100644" || e.Mode == "100755")
	}
	return m.Kind == "rename" && m.valid() && regular(m.Base) && regular(m.Head)
}
func associationID(current Run, f Finding, prior Run, old Finding, m GitChangeMetadata) string {
	currentContexts, _ := json.Marshal(append([]ContextRepository{}, contextPolicyItems(current.Snapshot)...))
	priorContexts, _ := json.Marshal(append([]ContextRepository{}, contextPolicyItems(prior.Snapshot)...))
	raw, _ := json.Marshal(struct {
		Version                                              string
		CurrentRun, PriorRun                                 int64
		Finding, PriorFinding, Fingerprint, PriorFingerprint string
		Base, Head, PriorBase, PriorHead                     string
		Line, PriorLine                                      int
		Metadata                                             GitChangeMetadata
		Contexts, PriorContexts                              string
	}{"finding-association-v1", current.ID, prior.ID, f.ID, old.ID, f.Fingerprint, old.Fingerprint, current.BaseSHA, current.HeadSHA, prior.BaseSHA, prior.HeadSHA, f.Line, old.Line, m, string(currentContexts), string(priorContexts)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// A suggestion is a path-mapping hint plus exact source facts, never semantic
// equivalence or an instruction to inherit the prior security review.
func suggestFindingAssociations(current Run, priors []Run) FindingAssociations {
	out := FindingAssociations{Items: []FindingAssociation{}, Limitations: []string{"Git rename describes this task's BASE→HEAD; it does not establish prior HEAD ancestry or semantic risk equivalence. Human review of both versions is required. Decisions never inherit prior risk reviews."}}
	if !associationTerminal(current.Status) {
		out.Limitations = append(out.Limitations, "Current task is not terminal; suggestions are unavailable.")
		return out
	}
	mappings := map[string][]GitChangeMetadata{}
	metadataSeen := map[string]bool{}
	for _, m := range current.Result.MetadataChanges {
		if associationRename(m) && !metadataSeen[m.canonical()] {
			metadataSeen[m.canonical()] = true
			mappings[m.NewPath] = append(mappings[m.NewPath], m)
		}
	}
	if len(mappings) == 0 {
		out.Limitations = append(out.Limitations, "No supported server-recorded regular-file rename metadata; arbitrary moves are not inferred.")
		return out
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	type priorKey struct {
		Run     int64
		Finding string
	}
	priorCounts := map[priorKey]int{}
	invalidIdentity := false
	checkIdentities := func(run Run) {
		for _, f := range run.Result.Findings {
			if fp := findingFingerprint(run.Snapshot, f); fp != "" && f.Fingerprint != fp {
				invalidIdentity = true
			}
		}
	}
	checkIdentities(current)
	for _, prior := range priors {
		if !associationScope(current, prior) {
			continue
		}
		checkIdentities(prior)
		priorPaths := map[string][]Finding{}
		for _, old := range prior.Result.Findings {
			if associationLine(prior.Snapshot, old) {
				priorPaths[old.File] = append(priorPaths[old.File], old)
			}
		}
		for _, f := range current.Result.Findings {
			if !associationLine(current.Snapshot, f) {
				continue
			}
			for _, m := range mappings[f.File] {
				for _, old := range priorPaths[m.OldPath] {
					if f.Type != old.Type || f.Evidence != old.Evidence || f.Trigger != old.Trigger {
						continue
					}
					id := associationID(current, f, prior, old, m)
					if seen[id] {
						continue
					}
					seen[id] = true
					out.Items = append(out.Items, FindingAssociation{ID: id, FindingID: f.ID, PriorRunID: prior.ID, PriorFindingID: old.ID, OldPath: old.File, NewPath: f.File, PriorHeadSHA: prior.HeadSHA, HeadSHA: current.HeadSHA, RenameBaseSHA: current.BaseSHA, Metadata: m, Evidence: f.Evidence, Trigger: f.Trigger, RiskType: f.Type, State: FindingAssociationDecision{Decision: "pending"}, History: []FindingAssociationDecision{}})
					counts[f.ID]++
					priorCounts[priorKey{prior.ID, old.ID}]++
				}
			}
		}
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.FindingID != b.FindingID {
			return a.FindingID < b.FindingID
		}
		if a.PriorRunID != b.PriorRunID {
			return a.PriorRunID > b.PriorRunID
		}
		return a.ID < b.ID
	})
	for i := range out.Items {
		out.Items[i].Ambiguous = counts[out.Items[i].FindingID] > 1 || priorCounts[priorKey{out.Items[i].PriorRunID, out.Items[i].PriorFindingID}] > 1
	}
	if len(out.Items) > associationMaxItems {
		out.Items = out.Items[:associationMaxItems]
		out.Truncated = true
		out.Limitations = append(out.Limitations, "Only the first 50 association suggestions are shown.")
	}
	if invalidIdentity {
		out.Limitations = append(out.Limitations, "Findings without a valid unique source fingerprint were omitted; inspect original anchors manually.")
	}
	return out
}
