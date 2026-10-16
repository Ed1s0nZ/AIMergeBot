package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestBoundedLosslessGitOracle(t *testing.T) {
	root := t.TempDir()
	for _, c := range namedCases() {
		t.Run(c[0], func(t *testing.T) {
			body, err := boundedRender(context.Background(), c[1], c[2], &matchWork{remaining: 4_000_000})
			if err != nil || !gitApply(root, c[1], c[2], body) {
				t.Fatalf("exact Git replay failed: %v", err)
			}
		})
	}
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
		body, err := boundedRender(context.Background(), base, head, &matchWork{remaining: 4_000_000})
		if err != nil || !gitApply(root, base, head, body) {
			t.Fatalf("fixed-seed exact replay %d: %v", i, err)
		}
	}
	base, head := namedCases()[4][1], namedCases()[4][2]
	body, err := boundedRender(context.Background(), base, head, &matchWork{remaining: 4_000_000})
	if err != nil {
		t.Fatal(err)
	}
	shifted := strings.Replace(body, "@@ -1,3 +1,3 @@", "@@ -2,3 +2,3 @@", 1)
	if shifted == body || !gitApplyVariant(root, base, head, shifted, true) || gitApply(root, base, head, shifted) {
		t.Fatal("offset rejection oracle must detect deliberately wrong coordinates")
	}
}

func TestBoundedPreciseAndAdversarial(t *testing.T) {
	root := t.TempDir()
	base := strings.Repeat("same\n", 999) + "old\n" + strings.Repeat("same\n", 999)
	head := strings.Repeat("same\n", 999) + "new\n" + strings.Repeat("same\n", 999)
	work := &matchWork{remaining: 4_000_000}
	body, err := boundedRender(context.Background(), base, head, work)
	added, removed := anchors(body)
	if err != nil || fmt.Sprint(added) != "[1000]" || fmt.Sprint(removed) != "[1000]" || work.charged != 1 || !gitApply(root, base, head, body) {
		t.Fatalf("repetitive one-line edit: %v; cells=%d", err, work.charged)
	}
	longBase, longHead := strings.Repeat(strings.Repeat("x", 118)+"a\n", 2000), strings.Repeat(strings.Repeat("x", 118)+"b\n", 2000)
	var forward, reverse strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&forward, "%04d-%s\n", i, strings.Repeat("p", 110))
		fmt.Fprintf(&reverse, "%04d-%s\n", 1999-i, strings.Repeat("p", 110))
	}
	cases := [][3]string{
		{"long_common_prefix_different_lines", longBase, longHead},
		{"reversed_unique_long_lines", forward.String(), reverse.String()},
		{"alternating", strings.Repeat("a\nb\n", 1000), strings.Repeat("b\na\n", 1000)},
		{"separated_hunks", "old\n" + strings.Repeat("same\n", 20) + "old\n", "new\n" + strings.Repeat("same\n", 20) + "new\n"},
	}
	for _, c := range cases {
		t.Run(c[0], func(t *testing.T) {
			work := &matchWork{remaining: 4_000_000}
			start := time.Now()
			body, err := boundedRender(context.Background(), c[1], c[2], work)
			elapsed := time.Since(start)
			if err != nil || work.charged > 4_000_000 || !gitApply(root, c[1], c[2], body) {
				t.Fatalf("bounded adversarial replay: %v", err)
			}
			t.Logf("cells_charged=%d elapsed_microseconds=%d", work.charged, elapsed.Microseconds())
		})
	}
}

// Independent shortest insert/delete oracle over tiny strings, using recursive
// memoized edit distance rather than the candidate's bottom-up LCS table.
func editDistance(a, b string) int {
	type pair struct{ a, b string }
	memo := map[pair]int{}
	var visit func(string, string) int
	visit = func(a, b string) int {
		key := pair{a, b}
		if value, ok := memo[key]; ok {
			return value
		}
		var result int
		switch {
		case len(a) == 0:
			result = len(b)
		case len(b) == 0:
			result = len(a)
		case a[0] == b[0]:
			result = visit(a[1:], b[1:])
		default:
			result = 1 + min(visit(a[1:], b), visit(a, b[1:]))
		}
		memo[key] = result
		return result
	}
	return visit(a, b)
}

func TestBoundedExhaustiveMinimalScripts(t *testing.T) {
	values := []string{""}
	for length := 1; length <= 5; length++ {
		for n := 0; n < 1<<length; n++ {
			var value strings.Builder
			for bit := 0; bit < length; bit++ {
				value.WriteByte('a' + byte(n>>bit&1))
			}
			values = append(values, value.String())
		}
	}
	lines := func(raw string) []string {
		out := make([]string, len(raw))
		for i := range raw {
			out[i] = raw[i : i+1]
		}
		return out
	}
	for _, a := range values {
		for _, b := range values {
			ops, err := boundedOps(context.Background(), lines(a), lines(b), &matchWork{remaining: 4_000_000})
			if err != nil {
				t.Fatal(err)
			}
			i, j, edits := 0, 0, 0
			var reconstructed strings.Builder
			for _, op := range ops {
				if op.I1 != i || op.J1 != j || op.I2 < i || op.J2 < j || op.I2 > len(a) || op.J2 > len(b) {
					t.Fatal("noncontiguous script")
				}
				switch op.Tag {
				case 'e':
					if a[i:op.I2] != b[j:op.J2] {
						t.Fatal("unequal equality")
					}
					reconstructed.WriteString(a[i:op.I2])
				case 'd':
					if j != op.J2 {
						t.Fatal("deletion consumed target")
					}
					edits += op.I2 - i
				case 'i':
					if i != op.I2 {
						t.Fatal("insertion consumed source")
					}
					edits += op.J2 - j
					reconstructed.WriteString(b[j:op.J2])
				default:
					t.Fatal("unknown opcode")
				}
				i, j = op.I2, op.J2
			}
			if i != len(a) || j != len(b) || reconstructed.String() != b || edits != editDistance(a, b) {
				t.Fatalf("nonminimal or incorrect script for %q -> %q", a, b)
			}
		}
	}
	t.Logf("exhaustive_pairs=%d", len(values)*len(values))
}

func TestBoundedMidMatchCancelAndSharedAllowance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	work := &matchWork{remaining: 4_000_000, observe: func(charged uint64) {
		if charged >= 8_000 {
			cancel()
		}
	}}
	body, err := boundedRender(ctx, strings.Repeat("a\n", 2000), strings.Repeat("b\n", 2000), work)
	if !errors.Is(err, context.Canceled) || body != "" || work.charged != 8_000 {
		t.Fatalf("in-progress cancellation must return no patch: %v; %d", err, work.charged)
	}
	shared := &matchWork{remaining: 12}
	for i := 0; i < 2; i++ {
		body, err = boundedRender(context.Background(), "a\nb\nc\n", "x\ny\nz\n", shared)
		if i == 0 && (err != nil || body == "") || i == 1 && (!errors.Is(err, errWork) || body != "") {
			t.Fatalf("file %d shared allowance result: %v", i, err)
		}
	}
	if shared.charged != 12 || shared.remaining != 0 {
		t.Fatal("work must not reset or exceed shared allowance")
	}
	_, err = boundedRender(ctx, "a\n", "b\n", shared)
	if !errors.Is(err, context.Canceled) || shared.charged != 12 {
		t.Fatal("pre-cancel must not spend additional work")
	}
}

func TestBoundedRejectsInvalidAndOversized(t *testing.T) {
	for _, bad := range []string{strings.Repeat("x", (256<<10)+1), strings.Repeat("x\n", 2001), "a\x00b", "\xff"} {
		body, err := boundedRender(context.Background(), bad, "new", &matchWork{remaining: 4_000_000})
		if !errors.Is(err, errLimit) || body != "" {
			t.Fatal("invalid or oversized input returned a patch")
		}
	}
	// Both inputs fit 256 KiB, but per-line diff prefixes exceed 512 KiB.
	base := strings.Repeat(strings.Repeat("a", 130)+"\n", 2000)
	head := strings.Repeat(strings.Repeat("b", 130)+"\n", 2000)
	body, err := boundedRender(context.Background(), base, head, &matchWork{remaining: 4_000_000})
	if !errors.Is(err, errLimit) || body != "" {
		t.Fatalf("output limit must discard partial hunk: %v", err)
	}
}

func TestBoundedRunWorkDoesNotResetAcrossFiles(t *testing.T) {
	work := &matchWork{remaining: 32_000_000}
	base, head := strings.Repeat("a\n", 2000), strings.Repeat("b\n", 2000)
	for file := 0; file < 9; file++ {
		body, err := boundedRender(context.Background(), base, head, work)
		if file < 8 && (err != nil || body == "") {
			t.Fatalf("file %d unexpectedly failed: %v", file, err)
		}
		if file == 8 && (!errors.Is(err, errWork) || body != "") {
			t.Fatal("ninth file must not acquire a fresh matching budget")
		}
	}
	if work.charged != 32_000_000 || work.remaining != 0 {
		t.Fatal("entire synthetic batch exceeded work allowance")
	}
}

func BenchmarkBoundedLCSWorstCase(b *testing.B) {
	base := strings.Repeat(strings.Repeat("x", 118)+"a\n", 2000)
	head := strings.Repeat(strings.Repeat("x", 118)+"b\n", 2000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := boundedRender(context.Background(), base, head, &matchWork{remaining: 4_000_000}); err != nil {
			b.Fatal(err)
		}
	}
}
