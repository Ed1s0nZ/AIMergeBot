package main

import (
	"context"
	"errors"

	"github.com/pmezard/go-difflib/difflib"
)

var errWork = errors.New("text matcher work allowance exhausted")

// Research-only single-owner allowance. It is not a durable production ledger.
type matchWork struct {
	remaining uint64
	charged   uint64
	// A synchronous observer permits deterministic cancellation during the DP.
	// Normal experiments leave it nil; it never runs in a separate goroutine.
	observe func(uint64)
}

func (w *matchWork) row(ctx context.Context, count uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil || count > w.remaining {
		return errWork
	}
	w.remaining -= count
	w.charged += count
	if w.observe != nil {
		w.observe(w.charged)
	}
	return ctx.Err()
}

func boundedRender(ctx context.Context, base, head string, work *matchWork) (string, error) {
	a, b, err := textTokens(ctx, base, head)
	if err != nil {
		return "", err
	}
	ops, err := boundedOps(ctx, a, b, work)
	if err != nil {
		return "", err
	}
	return renderGroups(ctx, a, b, contextGroups(ops))
}

// Each DP cell compares compact exact-line IDs. Bounds are checked before
// allocating; context is checked before every at-most-2000-cell row and step.
func boundedOps(ctx context.Context, a, b []string, work *matchWork) ([]difflib.OpCode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(a) > 2000 || len(b) > 2000 || work == nil {
		return nil, errLimit
	}
	ids := make(map[string]uint32, len(a)+len(b))
	intern := func(lines []string) ([]uint32, error) {
		out := make([]uint32, len(lines))
		for i, raw := range lines {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			id, exists := ids[raw]
			if !exists {
				id = uint32(len(ids) + 1)
				ids[raw] = id
			}
			out[i] = id
		}
		return out, nil
	}
	x, err := intern(a)
	if err != nil {
		return nil, err
	}
	y, err := intern(b)
	if err != nil {
		return nil, err
	}
	prefix := 0
	for prefix < len(x) && prefix < len(y) && x[prefix] == y[prefix] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prefix++
	}
	xEnd, yEnd := len(x), len(y)
	for xEnd > prefix && yEnd > prefix && x[xEnd-1] == y[yEnd-1] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		xEnd--
		yEnd--
	}
	n, m := xEnd-prefix, yEnd-prefix
	if n > 0 && m > 0 && work.remaining < uint64(m) {
		return nil, errWork
	}
	// uint16 is sufficient for a common subsequence of at most 2000 lines.
	width := m + 1
	table := make([]uint16, (n+1)*width)
	for i := n - 1; i >= 0; i-- {
		if err := work.row(ctx, uint64(m)); err != nil {
			return nil, err
		}
		for j := m - 1; j >= 0; j-- {
			position := i*width + j
			if x[prefix+i] == y[prefix+j] {
				table[position] = 1 + table[position+width+1]
			} else {
				table[position] = max(table[position+width], table[position+1])
			}
		}
	}
	ops := make([]difflib.OpCode, 0, n+m+2)
	appendOp := func(tag byte, i1, i2, j1, j2 int) {
		if i1 == i2 && j1 == j2 {
			return
		}
		if len(ops) > 0 {
			last := &ops[len(ops)-1]
			if last.Tag == tag && last.I2 == i1 && last.J2 == j1 {
				last.I2, last.J2 = i2, j2
				return
			}
		}
		ops = append(ops, difflib.OpCode{Tag: tag, I1: i1, I2: i2, J1: j1, J2: j2})
	}
	appendOp('e', 0, prefix, 0, prefix)
	i, j := 0, 0
	for i < n || j < m {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		aPos, bPos := prefix+i, prefix+j
		switch {
		case i < n && j < m && x[aPos] == y[bPos]:
			appendOp('e', aPos, aPos+1, bPos, bPos+1)
			i++
			j++
		case i < n && (j == m || table[(i+1)*width+j] >= table[i*width+j+1]):
			appendOp('d', aPos, aPos+1, bPos, bPos)
			i++
		default:
			appendOp('i', aPos, aPos, bPos, bPos+1)
			j++
		}
	}
	appendOp('e', xEnd, len(x), yEnd, len(y))
	return ops, nil
}

// Retain three context lines on each side, splitting only unchanged gaps
// longer than six lines. No library heuristic participates in these groups.
func contextGroups(ops []difflib.OpCode) [][]difflib.OpCode {
	const contextLines = 3
	var groups [][]difflib.OpCode
	var group []difflib.OpCode
	hasChange := false
	for _, code := range ops {
		if code.Tag != 'e' {
			group = append(group, code)
			hasChange = true
			continue
		}
		n := code.I2 - code.I1
		if len(group) == 0 {
			code.I1 = max(code.I1, code.I2-contextLines)
			code.J1 = max(code.J1, code.J2-contextLines)
			group = append(group, code)
		} else if n > contextLines*2 {
			left := code
			left.I2, left.J2 = left.I1+contextLines, left.J1+contextLines
			group = append(group, left)
			if hasChange {
				groups = append(groups, group)
			}
			code.I1, code.J1 = code.I2-contextLines, code.J2-contextLines
			group = []difflib.OpCode{code}
			hasChange = false
		} else {
			group = append(group, code)
		}
	}
	if hasChange {
		if last := &group[len(group)-1]; last.Tag == 'e' {
			last.I2 = min(last.I2, last.I1+contextLines)
			last.J2 = min(last.J2, last.J1+contextLines)
		}
		groups = append(groups, group)
	}
	return groups
}
