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
// If the streams cannot be aligned (they should not diverge in practice),
// Format returns an error so the caller leaves the file untouched rather than
// risk a mis-placed `!`.
func Format(filename string, src []byte) ([]byte, error) {
	result := preproc.Process(filename, src)
	if result == nil || result.Clean == nil {
		return nil, fmt.Errorf("preprocess failed")
	}

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

// reinsertBangs inserts '!' into formatted immediately before every token that
// was marked (by clean byte offset) in nonNil.
func reinsertBangs(clean []byte, nonNil map[int]bool, formatted []byte) ([]byte, error) {
	ct := significantTokens(clean)
	ft := significantTokens(formatted)

	insertAt := make([]int, 0, len(nonNil))
	i, j := 0, 0
	for i < len(ct) && j < len(ft) {
		if ct[i].kind == ft[j].kind {
			if nonNil[ct[i].offset] {
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
		return nil, fmt.Errorf("cannot re-insert !: token streams diverge at clean %s / formatted %s", ct[i].kind, ft[j].kind)
	}
	// Any marked token left unconsumed means we lost alignment.
	for ; i < len(ct); i++ {
		if ct[i].kind == token.COMMA {
			continue
		}
		return nil, fmt.Errorf("cannot re-insert !: %d clean tokens unmatched", len(ct)-i)
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
