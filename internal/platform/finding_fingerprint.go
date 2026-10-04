package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// findingFingerprint is a conservative exact-source identity, not semantic
// equivalence. Existing per-attempt IDs and human decisions remain independent.
func findingFingerprint(s Snapshot, f Finding) string {
	if s.ProjectID <= 0 || s.MRIID <= 0 || !commitID.MatchString(s.BaseSHA) || !commitID.MatchString(s.HeadSHA) || !validPath(f.File) || f.Evidence == "" || f.Trigger == "" || f.Type == "" {
		return ""
	}
	source := s.SourceProjectID
	if source == 0 {
		source = s.ProjectID
	}
	anchor := f.AnchorType
	if anchor == "" {
		anchor = "line"
	}
	side := f.Side
	if side == "" {
		side = "head"
	}
	evidence := f.Evidence
	if anchor == "git_metadata" {
		if f.Metadata == nil || !f.Metadata.valid() {
			return ""
		}
		evidence = f.Metadata.canonical()
	}
	raw, _ := json.Marshal(struct {
		Version                                     string
		Project, MR, Source                         int
		Side, File, Anchor, Type, Evidence, Trigger string
	}{"finding-fingerprint-v1", s.ProjectID, s.MRIID, source, side, f.File, anchor, f.Type, evidence, f.Trigger})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Repeated exact anchors cannot identify a unique issue without semantic context.
func clearAmbiguousFingerprints(findings []Finding) {
	counts := map[string]int{}
	for _, f := range findings {
		if f.Fingerprint != "" {
			counts[f.Fingerprint]++
		}
	}
	for i := range findings {
		if counts[findings[i].Fingerprint] > 1 {
			findings[i].Fingerprint = ""
		}
	}
}
