package fmt

import (
	"fmt"
	goformat "go/format"
	"go/scanner"
	"go/token"
	"sort"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Format returns formatted Gon source (v1.3 / M6a; alignment reworked v1.4.1).
//
// Pipeline: preprocess (strip !) → go/format on clean Go → re-insert ! at the
// type tokens that carried it.
//
// Re-insertion walks the clean and formatted token streams in lockstep.
// go/format only changes whitespace, comment layout, and trailing commas, so
// the two streams correspond token-for-token once comments, semicolons, and a
// gofmt-inserted/removed trailing comma are accounted for. The Nth marked type
// token in the clean stream is therefore the Nth corresponding token in the
// formatted stream — independent of how many times that type name appears
// textually elsewhere (the pre-v1.4.1 "nth textual match" heuristic corrupted
// files that used one type as both `!T` and `T`).
//
// One more change is tolerated, and only for a non-nil conversion (!T)(x):
// go/format removes redundant parentheses around a conversion target, so
// ((!*A))(x) is written (!*A)(x) (always one layer). A "(" in the run directly
// before a marked type, and the same number of ")" that close the run, may be
// missing from the formatted stream, and only when the remaining tokens then
// align one to one (RFC section 6.7, plan WP2). There is no generic tolerance for
// parentheses and no custom formatting pass: go/format stays the authority.
//
// If the streams cannot be aligned (they should not diverge in practice),
// Format returns an error so the caller leaves the file untouched rather than
// risk a mis-placed `!`.
func Format(filename string, src []byte) ([]byte, error) {
	result := preproc.Process(filename, src)

	formatted, err := goformat.Source(result.Clean)
	if err != nil {
		return nil, fmt.Errorf("format clean source: %w", err)
	}
	if len(result.NonNilOffsets) == 0 {
		return formatted, nil
	}

	return reinsertBangs(result.Clean, result.NonNilOffsets, formatted)
}

// tok is one significant token: comments and semicolons are dropped.
type tok struct {
	kind   token.Token
	offset int
}

// significantTokens scans src and returns its tokens with COMMENT and
// SEMICOLON removed. Auto-inserted semicolons and explicit ones both scan as
// SEMICOLON, so dropping them keeps clean and formatted aligned regardless of
// line breaks.
func significantTokens(src []byte) []tok {
	fset := token.NewFileSet()
	f := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(f, src, nil, 0) // no ScanComments

	var out []tok
	for {
		pos, kind, _ := s.Scan()
		if kind == token.EOF {
			break
		}
		if kind == token.SEMICOLON || kind == token.COMMENT {
			continue
		}
		out = append(out, tok{kind: kind, offset: fset.Position(pos).Offset})
	}
	return out
}

// parenGroup is the redundant-parenthesis allowance of one marked conversion
// target: the "(" and ")" that wrap the conversion's own parentheses. At most
// wrappers "(" and the same number of ")" may be missing from the formatted stream.
type parenGroup struct {
	wrappers     int
	skippedOpens int
	skippedClose int
}

// redundantParens returns, for the tokens of the clean stream that are a "(" or a
// ")" of a wrapper run, the group they belong to. A marked token at index t
// qualifies only when it follows a "(" whose closing ")" is followed by "(" after
// skipping wrapping ")" that close "(" directly before it: the shape
// ((!T))(x). Every "(" of the run and every ")" that closes it is eligible; which
// of them go/format drops does not matter, since they are the same token kind.
func redundantParens(ct []tok, marked []bool) (map[int]*parenGroup, map[int]*parenGroup) {
	opens := make(map[int]*parenGroup)
	closes := make(map[int]*parenGroup)
	for t := range ct {
		if !marked[t] || t == 0 || ct[t-1].kind != token.LPAREN {
			continue
		}
		open := t - 1
		closeIdx := matchingParen(ct, open)
		if closeIdx < 0 {
			continue
		}
		g := &parenGroup{}
		openList := []int{open}
		closeList := []int{closeIdx}
		ok := false
		for {
			n := closeIdx + 1
			if n >= len(ct) {
				break
			}
			if ct[n].kind == token.LPAREN {
				ok = true
				break
			}
			if ct[n].kind != token.RPAREN || open == 0 || ct[open-1].kind != token.LPAREN {
				break
			}
			open--
			closeIdx = n
			openList = append(openList, open)
			closeList = append(closeList, closeIdx)
			g.wrappers++
		}
		if !ok || g.wrappers == 0 {
			continue
		}
		for _, o := range openList {
			opens[o] = g
		}
		for _, c := range closeList {
			closes[c] = g
		}
	}
	return opens, closes
}

// matchingParen returns the index of the ")" that closes the "(" at openIdx, or
// -1 when the parentheses do not balance.
func matchingParen(ct []tok, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(ct); i++ {
		switch ct[i].kind {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// reinsertBangs inserts '!' into formatted immediately before every token that
// was marked (by clean byte offset) in nonNil.
func reinsertBangs(clean []byte, nonNil map[int]bool, formatted []byte) ([]byte, error) {
	ct := significantTokens(clean)
	ft := significantTokens(formatted)

	marked := make([]bool, len(ct))
	for k, t := range ct {
		marked[k] = nonNil[t.offset]
	}
	opens, closes := redundantParens(ct, marked)

	insertAt := make([]int, 0, len(nonNil))
	i, j := 0, 0
	for i < len(ct) && j < len(ft) {
		if ct[i].kind == ft[j].kind {
			if marked[i] {
				insertAt = append(insertAt, ft[j].offset)
			}
			i++
			j++
			continue
		}
		// The only token-stream change go/format makes is a trailing comma it
		// adds (single-line → multi-line) or removes (the reverse). Skip it on
		// whichever side has it and re-compare.
		if ct[i].kind == token.COMMA {
			i++
			continue
		}
		if ft[j].kind == token.COMMA {
			j++
			continue
		}
		// go/format removes redundant parentheses around a conversion target.
		if g := opens[i]; g != nil && ct[i].kind == token.LPAREN && g.skippedOpens < g.wrappers {
			g.skippedOpens++
			i++
			continue
		}
		if g := closes[i]; g != nil && ct[i].kind == token.RPAREN && g.skippedClose < g.wrappers {
			g.skippedClose++
			i++
			continue
		}
		return nil, fmt.Errorf("cannot re-insert !: token streams diverge at clean %s / formatted %s", ct[i].kind, ft[j].kind)
	}
	// Any marked token left unconsumed means we lost alignment.
	for ; i < len(ct); i++ {
		if ct[i].kind == token.COMMA {
			continue
		}
		return nil, fmt.Errorf("cannot re-insert !: %d clean tokens unmatched", len(ct)-i)
	}
	seen := make(map[*parenGroup]bool)
	for _, g := range opens {
		if !seen[g] {
			seen[g] = true
			if g.skippedOpens != g.skippedClose {
				return nil, fmt.Errorf("cannot re-insert !: unbalanced parentheses removed around a conversion")
			}
		}
	}
	if len(insertAt) == 0 {
		return nil, fmt.Errorf("cannot re-insert !: no marked token matched")
	}

	sort.Ints(insertAt)
	out := make([]byte, 0, len(formatted)+len(insertAt))
	prev := 0
	for _, at := range insertAt {
		if at < prev || at > len(formatted) {
			return nil, fmt.Errorf("cannot re-insert !: offset %d out of order", at)
		}
		out = append(out, formatted[prev:at]...)
		out = append(out, '!')
		prev = at
	}
	out = append(out, formatted[prev:]...)
	return out, nil
}
