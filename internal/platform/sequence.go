package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type SequenceParticipant struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type SequenceReference struct {
	AnchorType string             `json:"anchor_type,omitempty"`
	Metadata   *GitChangeMetadata `json:"metadata,omitempty"`
	Side       string             `json:"side"`
	File       string             `json:"file"`
	Line       int                `json:"line"`
	Snippet    string             `json:"snippet"`
	SHA        string             `json:"sha,omitempty"`
}
type SequenceStep struct {
	From      string              `json:"from"`
	To        string              `json:"to"`
	Label     string              `json:"label"`
	Kind      string              `json:"kind"`
	Certainty string              `json:"certainty"`
	Risk      bool                `json:"risk"`
	Evidence  []SequenceReference `json:"evidence"`
}
type SequenceInput struct {
	Participants []SequenceParticipant `json:"participants"`
	Steps        []SequenceStep        `json:"steps"`
	Limitations  []string              `json:"limitations"`
}
type SequenceDiagram struct {
	SequenceInput
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Mermaid string `json:"mermaid,omitempty"`
}

var sequenceID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

func ParseSequence(raw string) (SequenceInput, error) {
	var input SequenceInput
	if len(raw) > 32*1024 {
		return input, fmt.Errorf("sequence output exceeds budget")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&input); e != nil {
		return input, fmt.Errorf("invalid sequence JSON")
	}
	var tail any
	if e := decoder.Decode(&tail); e != io.EOF {
		return input, fmt.Errorf("trailing sequence data")
	}
	return input, nil
}
func boundedLabel(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}

func ValidateSequence(ctx context.Context, repo Repository, snap Snapshot, f Finding, input SequenceInput) (*SequenceDiagram, error) {
	if len(input.Participants) < 2 || len(input.Participants) > 8 || len(input.Steps) < 1 || len(input.Steps) > 16 || input.Limitations == nil || len(input.Limitations) > 8 {
		return nil, fmt.Errorf("invalid sequence size or missing limitations array")
	}
	ids := map[string]bool{}
	for _, p := range input.Participants {
		if !sequenceID.MatchString(p.ID) || ids[p.ID] || !boundedLabel(p.Label, 48) {
			return nil, fmt.Errorf("invalid sequence participant")
		}
		ids[p.ID] = true
	}
	for _, v := range input.Limitations {
		if !boundedLabel(v, 240) {
			return nil, fmt.Errorf("invalid sequence limitation")
		}
	}
	status := "ready"
	if len(input.Limitations) > 0 {
		status = "partial"
	}
	anchored := false
	cache := map[string][]string{}
	for i := range input.Steps {
		step := &input.Steps[i]
		if !ids[step.From] || !ids[step.To] || !boundedLabel(step.Label, 140) || len(step.Evidence) > 4 {
			return nil, fmt.Errorf("invalid sequence step")
		}
		if step.Kind != "call" && step.Kind != "return" && step.Kind != "note" {
			return nil, fmt.Errorf("invalid sequence step kind")
		}
		if step.Certainty != "cited" && step.Certainty != "inferred" {
			return nil, fmt.Errorf("invalid sequence certainty")
		}
		if step.Certainty == "cited" && len(step.Evidence) == 0 {
			return nil, fmt.Errorf("cited sequence step needs code references")
		}
		if step.Certainty == "inferred" {
			status = "partial"
		}
		for n := range step.Evidence {
			ref := &step.Evidence[n]
			if ref.Side != "head" && ref.Side != "base" || !validPath(ref.File) || !boundedLabel(ref.Snippet, 500) {
				return nil, fmt.Errorf("invalid sequence reference")
			}
			sha := snap.HeadSHA
			if ref.Side == "base" {
				sha = snap.BaseSHA
			}
			if ref.SHA != "" && ref.SHA != sha {
				return nil, fmt.Errorf("sequence citation SHA mismatch")
			}
			ref.SHA = sha
			if ref.AnchorType == "git_metadata" {
				if f.AnchorType != "git_metadata" || f.Metadata == nil || step.Kind != "note" || ref.Line != 0 || ref.Side != f.Side || ref.File != f.File || ref.Snippet != f.Evidence || ref.Metadata != nil && ref.Metadata.canonical() != f.Metadata.canonical() {
					return nil, fmt.Errorf("metadata citation must annotate the primary pinned change")
				}
				copy := *f.Metadata
				ref.Metadata = &copy
				if step.Risk {
					anchored = true
				}
				status = "partial"
				continue
			}
			if ref.Line < 1 || ref.AnchorType != "" && ref.AnchorType != "line" || ref.Metadata != nil {
				return nil, fmt.Errorf("invalid sequence code anchor")
			}
			if ref.Side == "base" && step.Kind != "note" {
				return nil, fmt.Errorf("base evidence must annotate before-change code, not a current call")
			}
			key := ref.Side + ":" + ref.File
			lines, ok := cache[key]
			if !ok {
				content, e := repo.ReadFile(ctx, snap, ref.File, ref.Side == "base")
				if e != nil {
					return nil, fmt.Errorf("cannot read sequence citation %s", ref.File)
				}
				lines = strings.Split(content, "\n")
				cache[key] = lines
			}
			if ref.Line > len(lines) || !strings.Contains(lines[ref.Line-1], ref.Snippet) {
				return nil, fmt.Errorf("sequence evidence does not match %s:%d", ref.File, ref.Line)
			}
			side := f.Side
			if side == "" {
				side = "head"
			}
			if step.Risk && ref.Side == side && ref.File == f.File && ref.Line == f.Line && strings.Contains(ref.Snippet, f.Evidence) {
				anchored = true
			}
		}
	}
	if !anchored {
		return nil, fmt.Errorf("sequence risk step must cite the primary finding")
	}
	diagram := &SequenceDiagram{SequenceInput: input, Status: status}
	diagram.Mermaid = SequenceMermaid(input)
	return diagram, nil
}

// Encode punctuation as Mermaid entities so labels cannot introduce directives, HTML or statements.
func mermaidText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			b.WriteRune(r)
		} else {
			fmt.Fprintf(&b, "#%d;", r)
		}
	}
	if b.String() == "end" {
		return "#101;nd"
	}
	return b.String()
}
func SequenceMermaid(input SequenceInput) string {
	var b strings.Builder
	b.WriteString("sequenceDiagram\n    autonumber\n")
	for _, p := range input.Participants {
		fmt.Fprintf(&b, "    participant %s as %s\n", p.ID, mermaidText(p.Label))
	}
	for _, s := range input.Steps {
		if s.Risk {
			b.WriteString("    rect rgb(255, 237, 237)\n")
		}
		label := s.Label
		if s.Certainty == "inferred" {
			label = "[推测] " + label
		}
		if s.Risk {
			label = "[风险] " + label
		}
		for _, ref := range s.Evidence {
			if ref.Side == "base" {
				label = "[变更前/已删除证据] " + label
				break
			}
		}
		if s.Kind == "note" {
			fmt.Fprintf(&b, "    Note over %s,%s: %s\n", s.From, s.To, mermaidText(label))
		} else {
			arrow := "->>"
			if s.Kind == "return" || s.Certainty == "inferred" {
				arrow = "-->>"
			}
			fmt.Fprintf(&b, "    %s%s%s: %s\n", s.From, arrow, s.To, mermaidText(label))
		}
		if s.Risk {
			b.WriteString("    end\n")
		}
	}
	return b.String()
}
