// Package preproc identifies and strips ! type modifiers from Gon source.
// It produces clean Go source and records where !T types appeared.
package preproc

import (
	"go/scanner"
	"go/token"
)

// Result holds the preprocessed source and metadata about ! type modifiers.
type Result struct {
	// Clean is the source with all ! type modifiers removed — valid Go.
	Clean []byte
	// NonNilOffsets is the set of byte offsets in Clean where a !T type begins.
	NonNilOffsets map[int]bool
}

// Process scans a Gon source file, identifies ! type modifiers, and returns
// the source with those ! stripped plus the positions where !T types began.
//
// Phase 1 rule: ! is a type modifier if the previous meaningful token is:
//   - IDENT  → covers: var x !T, func f(x !T), struct { X !T }, receiver (r !T)
//   - RPAREN → covers: func f() !T, func f(a int) !T
//
// This handles all Phase 1 test cases. COMMA and LPAREN cases (unnamed params,
// multi-return) are deferred to v1.1.
func Process(filename string, src []byte) *Result {
	fset := token.NewFileSet()
	file := fset.AddFile(filename, 1, len(src))

	var s scanner.Scanner
	// Suppress scanner errors — we'll catch real parse errors later.
	s.Init(file, src, nil, scanner.ScanComments)

	type tokInfo struct {
		offset int
		kind   token.Token
	}

	// Collect all tokens with their byte offsets.
	var tokens []tokInfo
	for {
		pos, kind, _ := s.Scan()
		tokens = append(tokens, tokInfo{
			offset: fset.Position(pos).Offset,
			kind:   kind,
		})
		if kind == token.EOF {
			break
		}
	}

	// Find ! tokens that are type modifiers.
	typeModifiers := make(map[int]bool) // byte offsets of ! type modifiers in src
	prevIdx := -1
	for i, t := range tokens {
		if t.kind == token.COMMENT {
			continue // skip comments when determining context
		}
		if t.kind == token.NOT {
			if prevIdx >= 0 {
				prev := tokens[prevIdx]
				if prev.kind == token.IDENT || prev.kind == token.RPAREN {
					typeModifiers[t.offset] = true
				}
			}
		}
		prevIdx = i
	}

	// Build clean source by removing ! type modifiers.
	// Track the clean-source offset where each removed !T type begins.
	clean := make([]byte, 0, len(src))
	nonNilOffsets := make(map[int]bool)

	for origOff := 0; origOff < len(src); origOff++ {
		if typeModifiers[origOff] {
			// This ! is a type modifier. The next character is the start
			// of the type in clean source — record that clean offset.
			nonNilOffsets[len(clean)] = true
			continue // don't copy ! into clean source
		}
		clean = append(clean, src[origOff])
	}

	return &Result{
		Clean:         clean,
		NonNilOffsets: nonNilOffsets,
	}
}
