package platform

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// deterministicFormattingOrigin marks findings produced by the server-side
// formatting-scope analysis instead of the audit model. They are deterministic
// lexical facts about the pinned diff, carry no security claim, and are
// excluded from security verification, claim review and sequence diagrams.
const deterministicFormattingOrigin = "formatting_scope"

// maxFormattingScopeFindings bounds deterministic findings per audit; further
// violating files remain an explicit coverage note.
const maxFormattingScopeFindings = 20

// maxFormattingScopeExamples bounds recorded anchor examples per file.
const maxFormattingScopeExamples = 3

type FormattingScopeExample struct {
	HeadLine int    `json:"head_line"`
	Text     string `json:"text"`
}

// FileFormattingScope is a language-independent whitespace-only statistic for
// one included file. Lines compare equal when removing every whitespace run
// makes them identical; whitespace inside string literals cannot be
// distinguished in this lexical view.
type FileFormattingScope struct {
	Path            string                   `json:"path"`
	Hunks           int                      `json:"hunks"`
	FormattingHunks int                      `json:"formatting_hunks"`
	FormattingLines int                      `json:"formatting_lines"`
	AddedLines      int                      `json:"added_lines"`
	RemovedLines    int                      `json:"removed_lines"`
	Examples        []FormattingScopeExample `json:"examples,omitempty"`
}

// Violating reports whether some hunk changed nothing but whitespace: a region
// with no content modification that was nevertheless reformatted.
func (s FileFormattingScope) Violating() bool { return s.FormattingHunks > 0 }

// mergeFile accumulates stats for the same path across diff chunks or scopes.
func (s *FileFormattingScope) mergeFile(other FileFormattingScope) {
	s.Hunks += other.Hunks
	s.FormattingHunks += other.FormattingHunks
	s.FormattingLines += other.FormattingLines
	s.AddedLines += other.AddedLines
	s.RemovedLines += other.RemovedLines
	s.Examples = append(s.Examples, other.Examples...)
	sort.SliceStable(s.Examples, func(i, j int) bool { return s.Examples[i].HeadLine < s.Examples[j].HeadLine })
	if len(s.Examples) > maxFormattingScopeExamples {
		s.Examples = s.Examples[:maxFormattingScopeExamples]
	}
}

func stripWhitespace(line string) string {
	return strings.Join(strings.Fields(line), "")
}

type formattingBucket struct {
	raw   map[string]int
	total int
}

func (b *formattingBucket) add(line string) {
	if b.raw == nil {
		b.raw = map[string]int{}
	}
	b.raw[line]++
	b.total++
}

type formattingAdded struct {
	raw     string
	display string
	line    int
}

// changedExample reports the first added line that actually changed: raw-equal
// pairs are consumed first so the anchor never lands on a line git re-emitted
// unchanged inside a formatting-only hunk.
func changedExample(removed []string, added []formattingAdded) (FormattingScopeExample, bool) {
	queues := map[string][]int{}
	for j, entry := range added {
		queues[entry.raw] = append(queues[entry.raw], j)
	}
	consumed := make([]bool, len(added))
	for _, line := range removed {
		if q := queues[line]; len(q) > 0 {
			consumed[q[0]] = true
			queues[line] = q[1:]
		}
	}
	for j, entry := range added {
		if !consumed[j] && strings.TrimSpace(entry.display) != "" {
			return FormattingScopeExample{HeadLine: entry.line, Text: entry.display}, true
		}
	}
	return FormattingScopeExample{}, false
}

// formattingScopeForDiff classifies one included textual diff. ok is false when
// no valid hunk was found. Raw line endings are kept for comparisons so a
// CRLF→LF rewrite counts as a whitespace-only difference.
func formattingScopeForDiff(textual string) (FileFormattingScope, bool) {
	stat := FileFormattingScope{}
	removed := map[string]*formattingBucket{}
	added := map[string]*formattingBucket{}
	removedRaw := []string{}
	addedList := []formattingAdded{}
	newLine := 0
	inHunk := false
	flush := func() {
		if len(removedRaw) == 0 && len(addedList) == 0 {
			removed = map[string]*formattingBucket{}
			added = map[string]*formattingBucket{}
			return
		}
		stat.Hunks++
		stat.RemovedLines += len(removedRaw)
		stat.AddedLines += len(addedList)
		matched, rawSame := 0, 0
		for key, r := range removed {
			a := added[key]
			if a == nil {
				continue
			}
			matched += min(r.total, a.total)
			for raw, count := range r.raw {
				rawSame += min(count, a.raw[raw])
			}
		}
		stat.FormattingLines += matched - rawSame
		if matched > rawSame && matched == len(removedRaw) && matched == len(addedList) {
			stat.FormattingHunks++
			if example, ok := changedExample(removedRaw, addedList); ok {
				stat.Examples = append(stat.Examples, example)
			}
		}
		removed = map[string]*formattingBucket{}
		added = map[string]*formattingBucket{}
		removedRaw = nil
		addedList = nil
	}
	bucket := func(lines map[string]*formattingBucket, line string) {
		key := stripWhitespace(line)
		b := lines[key]
		if b == nil {
			b = &formattingBucket{}
			lines[key] = b
		}
		b.add(line)
	}
	for _, line := range strings.Split(textual, "\n") {
		if m := hunkPattern.FindStringSubmatch(line); m != nil {
			flush()
			newLine, _ = strconv.Atoi(m[2])
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case '+':
			text := line[1:]
			bucket(added, text)
			addedList = append(addedList, formattingAdded{raw: text, display: strings.TrimRight(text, "\r"), line: newLine})
			newLine++
		case ' ':
			newLine++
		case '-':
			text := line[1:]
			bucket(removed, text)
			removedRaw = append(removedRaw, text)
		case '\\':
		default:
			inHunk = false
			flush()
		}
	}
	flush()
	return stat, stat.Hunks > 0
}

// recordFormattingScopeFindings accepts one deterministic finding per violating
// included file and returns coverage notes for skipped or truncated results, so
// a detected violation is never silently dropped. It is a no-op unless the
// caller enabled the setting.
func (t *auditTools) recordFormattingScopeFindings(ctx context.Context) []string {
	notes := []string{}
	paths := make([]string, 0, len(t.scope.Formatting))
	for p := range t.scope.Formatting {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	recorded := 0
	for _, p := range paths {
		stat := t.scope.Formatting[p]
		if !stat.Violating() {
			continue
		}
		if recorded >= maxFormattingScopeFindings {
			notes = append(notes, fmt.Sprintf("Formatting scope findings truncated: first %d violating files kept; remaining files are unreported", maxFormattingScopeFindings))
			break
		}
		finding, ok := formattingScopeFinding(stat)
		if !ok {
			notes = append(notes, "Formatting scope finding skipped: no nonblank added line available as anchor in "+p)
			continue
		}
		if _, err := t.acceptFinding(ctx, finding); err != nil {
			notes = append(notes, "Formatting scope finding rejected: "+p+": "+err.Error())
			continue
		}
		recorded++
	}
	return notes
}

// formattingScopeFinding builds the deterministic low-severity finding anchored
// to the first nonblank added line of the file's first formatting-only hunk.
func formattingScopeFinding(stat FileFormattingScope) (Finding, bool) {
	for _, example := range stat.Examples {
		if example.HeadLine < 1 || strings.TrimSpace(example.Text) == "" {
			continue
		}
		return Finding{
			Origin:      deterministicFormattingOrigin,
			Side:        "head",
			File:        stat.Path,
			Line:        example.HeadLine,
			Severity:    "low",
			Type:        "formatting scope",
			Confidence:  "candidate",
			Evidence:    example.Text,
			Title:       fmt.Sprintf("格式化范围越界：%d 个区块仅含格式变更", stat.FormattingHunks),
			Description: fmt.Sprintf("确定性静态检测（语言无关的空白归一化比较）：该文件有 %d 个差异区块（hunk）的全部变更都是空白差异（整文件共 %d 行仅空白差异），这些区域没有内容修改，属于对本次改动之外的代码执行了格式化。团队规则要求只对修改部分执行格式化，无关格式变更会混入提交记录、影响追溯与审查。判定方法为逐行去除全部空白后做多重集比对；字符串字面量内部的空白差异在语言无关视图中无法区分。本项为确定性检测，不构成安全结论，不参与安全向独立复核与时序图。", stat.FormattingHunks, stat.FormattingLines),
			Trigger:     "提交中包含对未修改区域的格式化（例如 IDE 保存时自动格式化整个文件或类），使无关格式变更混入本次 diff。",
			Suggestion:  "还原该文件中与本次改动无关的格式变更，仅在修改部分内保留格式化；或将整体格式化拆分为独立的提交或 MR。",
		}, true
	}
	return Finding{}, false
}
