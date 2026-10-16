// Isolated candidate experiment: not imported by the production application.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
)

var errLimit = errors.New("text diff probe input/output unavailable")

func need(ok bool, label string) {
	if !ok {
		panic(label)
	}
}
func tokens(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.SplitAfter(raw, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
func unifiedRange(start, end int) string {
	count := end - start
	if count == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if count == 1 {
		return strconv.Itoa(start + 1)
	}
	return fmt.Sprintf("%d,%d", start+1, count)
}
func render(ctx context.Context, base, head string, auto bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(base) > 256<<10 || len(head) > 256<<10 || !utf8.ValidString(base) || !utf8.ValidString(head) || strings.ContainsRune(base, 0) || strings.ContainsRune(head, 0) {
		return "", errLimit
	}
	a, b := tokens(base), tokens(head)
	if len(a) > 2000 || len(b) > 2000 {
		return "", errLimit
	}
	matcher := difflib.NewMatcherWithJunk(a, b, auto, nil)
	groups := matcher.GetGroupedOpCodes(3)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var out strings.Builder
	write := func(s string) error {
		if len(s) > (512<<10)-out.Len() {
			return errLimit
		}
		out.WriteString(s)
		return nil
	}
	line := func(prefix, raw string) error {
		if err := write(prefix + raw); err != nil {
			return err
		}
		if !strings.HasSuffix(raw, "\n") {
			return write("\n\\ No newline at end of file\n")
		}
		return nil
	}
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if len(group) == 0 {
			return "", errLimit
		}
		first, last := group[0], group[len(group)-1]
		if err := write("@@ -" + unifiedRange(first.I1, last.I2) + " +" + unifiedRange(first.J1, last.J2) + " @@\n"); err != nil {
			return "", err
		}
		prevI, prevJ := first.I1, first.J1
		for _, code := range group {
			if code.I1 != prevI || code.J1 != prevJ || code.I1 < 0 || code.J1 < 0 || code.I2 < code.I1 || code.J2 < code.J1 || code.I2 > len(a) || code.J2 > len(b) {
				return "", errLimit
			}
			prevI, prevJ = code.I2, code.J2
			switch code.Tag {
			case 'e':
				if code.I2-code.I1 != code.J2-code.J1 {
					return "", errLimit
				}
				for i := 0; i < code.I2-code.I1; i++ {
					if a[code.I1+i] != b[code.J1+i] {
						return "", errLimit
					}
					if err := line(" ", a[code.I1+i]); err != nil {
						return "", err
					}
				}
			case 'r', 'd', 'i':
				if code.Tag == 'd' && code.J1 != code.J2 || code.Tag == 'i' && code.I1 != code.I2 {
					return "", errLimit
				}
				for _, raw := range a[code.I1:code.I2] {
					if err := line("-", raw); err != nil {
						return "", err
					}
				}
				for _, raw := range b[code.J1:code.J2] {
					if err := line("+", raw); err != nil {
						return "", err
					}
				}
			default:
				return "", errLimit
			}
		}
	}
	return out.String(), nil
}

var hunk = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func anchors(body string) (added, removed []int) {
	oldLine, newLine := 0, 0
	for _, line := range strings.Split(body, "\n") {
		if header := hunk.FindStringSubmatch(line); header != nil {
			oldLine, _ = strconv.Atoi(header[1])
			newLine, _ = strconv.Atoi(header[2])
			continue
		}
		if line == "" {
			continue
		}
		switch line[0] {
		case '+':
			added = append(added, newLine)
			newLine++
		case '-':
			removed = append(removed, oldLine)
			oldLine++
		case ' ':
			oldLine++
			newLine++
		}
	}
	return
}
func gitApply(root, base, head, body string) bool {
	return gitApplyVariant(root, base, head, body, false)
}

func gitApplyVariant(root, base, head, body string, allowOffset bool) bool {
	dir, err := os.MkdirTemp(root, "case-")
	need(err == nil, "temporary case")
	file := filepath.Join(dir, "evidence.txt")
	need(os.WriteFile(file, []byte(base), 0600) == nil, "write synthetic input")
	if body != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "--no-pager", "-c", "core.hooksPath=/dev/null", "apply", "--verbose", "-")
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
		cmd.WaitDelay = time.Second
		var diagnostic bytes.Buffer
		cmd.Stderr = &diagnostic
		cmd.Stdin = strings.NewReader("--- a/evidence.txt\n+++ b/evidence.txt\n" + body)
		if cmd.Run() != nil {
			return false
		}
		// Do not let Git's offset matching hide incorrect hunk coordinates.
		if !allowOffset && (strings.Contains(diagnostic.String(), "offset") || strings.Contains(diagnostic.String(), "fuzz")) {
			return false
		}
	}
	raw, err := os.ReadFile(file)
	return err == nil && string(raw) == head
}
func run() map[string]any {
	root, err := os.MkdirTemp("", "aim-text-diff-probe-")
	need(err == nil, "fixture root")
	defer os.RemoveAll(root)
	cases := [][3]string{
		{"empty", "", ""}, {"unchanged", "same\n", "same\n"}, {"add", "", "new\n"}, {"delete", "old\n", ""},
		{"replace", "first\nold\nlast\n", "first\nnew\nlast\n"}, {"no_final_lf", "old", "new"},
		{"only_add_lf", "same", "same\n"}, {"only_remove_lf", "same\n", "same"},
		{"crlf", "first\r\nold\r\nlast\r\n", "first\r\nnew\r\nlast\r\n"},
		{"line_endings", "same\r\n", "same\n"}, {"unicode", "第一行\n旧值\n", "第一行\n新值\n"},
		{"blank", "\n\nold\n", "\nnew\n\n"}, {"insert_last", "first\n", "first\nlast"},
	}
	for _, c := range cases {
		body, err := render(context.Background(), c[1], c[2], false)
		need(err == nil && gitApply(root, c[1], c[2], body), "lossless Git apply: "+c[0])
	}
	// A fixed corpus checks rendering independently through Git's patch parser.
	random := rand.New(rand.NewSource(1907))
	pool := []string{"same\n", "内容\n", "\n", " \r\n", "\tvalue\n", "+++ source\n", "@@ -8 +8 @@\n", "\\ marker\n"}
	text := func() string {
		var raw strings.Builder
		for n := random.Intn(40); n > 0; n-- {
			raw.WriteString(pool[random.Intn(len(pool))])
		}
		value := raw.String()
		if random.Intn(2) == 0 {
			value = strings.TrimSuffix(value, "\n")
		}
		return value
	}
	for i := 0; i < 64; i++ {
		base, head := text(), text()
		body, err := render(context.Background(), base, head, false)
		need(err == nil && gitApply(root, base, head, body), fmt.Sprintf("deterministic corpus case %d", i))
	}
	body, err := render(context.Background(), cases[4][1], cases[4][2], false)
	need(err == nil, "simple replacement")
	a, d := anchors(body)
	need(fmt.Sprint(a) == "[2]" && fmt.Sprint(d) == "[2]", "simple fixed side line anchors")
	shifted := strings.Replace(body, "@@ -1,3 +1,3 @@", "@@ -2,3 +2,3 @@", 1)
	need(shifted != body && gitApplyVariant(root, cases[4][1], cases[4][2], shifted, true) && !gitApply(root, cases[4][1], cases[4][2], shifted), "offset oracle positive and negative controls")
	repeatedBase := strings.Repeat("same\n", 999) + "old\n" + strings.Repeat("same\n", 999)
	repeatedHead := strings.Repeat("same\n", 999) + "new\n" + strings.Repeat("same\n", 999)
	defaultBody, err := render(context.Background(), repeatedBase, repeatedHead, true)
	need(err == nil && gitApply(root, repeatedBase, repeatedHead, defaultBody), "default matcher edit script")
	defaultAdded, defaultRemoved := anchors(defaultBody)
	start := time.Now()
	preciseBody, err := render(context.Background(), repeatedBase, repeatedHead, false)
	elapsed := time.Since(start)
	need(err == nil && gitApply(root, repeatedBase, repeatedHead, preciseBody), "no-autojunk repeated edit script")
	preciseAdded, preciseRemoved := anchors(preciseBody)
	need(fmt.Sprint(preciseAdded) == "[1000]" && fmt.Sprint(preciseRemoved) == "[1000]", "repeated-line edit must not mark unchanged tail")
	raw, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: tokens("old"), B: tokens("new"), Context: 3})
	need(err == nil, "raw library renderer")
	rawNoEOLApplies := gitApply(root, "old", "new", raw)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = render(ctx, "old", "new", false)
	need(errors.Is(err, context.Canceled), "pre-canceled match")
	for _, bad := range []string{strings.Repeat("x", (256<<10)+1), strings.Repeat("x\n", 2001), "a\x00b", "\xff"} {
		_, err = render(context.Background(), bad, "new", false)
		need(errors.Is(err, errLimit), "input budgets/text classification")
	}
	denseBase := strings.Repeat(strings.Repeat("x", 118)+"a\n", 2000)
	denseHead := strings.Repeat(strings.Repeat("x", 118)+"b\n", 2000)
	start = time.Now()
	dense, err := render(context.Background(), denseBase, denseHead, false)
	denseElapsed := time.Since(start)
	need(err == nil && gitApply(root, denseBase, denseHead, dense), "maximum repeated long-line replacement")
	return map[string]any{"synthetic_only": true, "library": "go-difflib/v1.0.0", "lossless_cases": len(cases), "deterministic_corpus_cases": 64, "simple_line_anchors": true, "autojunk_repeated_added": len(defaultAdded), "autojunk_repeated_removed": len(defaultRemoved), "no_autojunk_repeated_added": len(preciseAdded), "no_autojunk_repeated_removed": len(preciseRemoved), "no_autojunk_repeated_microseconds": elapsed.Microseconds(), "dense_2000_lines_microseconds": denseElapsed.Microseconds(), "raw_library_no_final_lf_replay_ok": rawNoEOLApplies, "pre_canceled_stops": true, "invalid_and_oversized_inputs_rejected": true, "mid_match_context_interrupt_supported": false, "production_adapter_tested": false}
}
func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, "text diff probe failed:", err)
			os.Exit(1)
		}
	}()
	need(json.NewEncoder(os.Stdout).Encode(run()) == nil, "JSON output")
}
