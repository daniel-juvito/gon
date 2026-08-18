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

type tokInfo struct {
	offset int
	kind   token.Token
}

// Process scans a Gon source file, identifies ! type modifiers, and returns
// the source with those ! stripped plus the positions where !T types began.
//
// A ! is a type modifier when it appears in a type position:
//
//   - after IDENT or RPAREN (var x !T, func f() !T, field X !T, …)
//   - after LPAREN or COMMA inside a function/method parameter list,
//     parenthesized result list, function type, or interface method signature
//     — covering multi-return forms such as func Open() (!*string, error)
//
// Unary ! in expressions (f(a, !b), (!flag), !ok) is left untouched.
func Process(filename string, src []byte) *Result {
	fset := token.NewFileSet()
	file := fset.AddFile(filename, 1, len(src))

	var s scanner.Scanner
	// Suppress scanner errors — we'll catch real parse errors later.
	s.Init(file, src, nil, scanner.ScanComments)

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

	// Mark tokens that lie inside parameter / parenthesized-result type lists.
	// Those lists are the only places where ! may follow LPAREN or COMMA as a
	// type modifier rather than unary NOT.
	inTypeList := markTypeListTokens(tokens)

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
				switch prev.kind {
				case token.IDENT, token.RPAREN:
					// Classic type positions: var x !T, func f() !T, …
					typeModifiers[t.offset] = true
				case token.LPAREN, token.COMMA:
					// Only when this LPAREN/COMMA is inside a signature type list.
					if inTypeList[prevIdx] {
						typeModifiers[t.offset] = true
					}
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

// markTypeListTokens returns a mask over tokens: true if the token is part of
// a function/method parameter list or parenthesized result list (or the same
// inside a function type / interface method).
func markTypeListTokens(tokens []tokInfo) []bool {
	mask := make([]bool, len(tokens))

	// Track interface body depth so method signatures without a leading
	// "func" keyword are still recognized.
	interfaceDepth := 0

	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if t.kind == token.COMMENT {
			i++
			continue
		}

		switch t.kind {
		case token.INTERFACE:
			j := nextNonComment(tokens, i+1)
			if j < len(tokens) && tokens[j].kind == token.LBRACE {
				interfaceDepth++
				i = j + 1
				continue
			}

		case token.RBRACE:
			if interfaceDepth > 0 {
				interfaceDepth--
			}

		case token.FUNC:
			end := markFuncSignature(tokens, mask, i+1)
			i = end
			continue

		case token.IDENT:
			// Interface method: Name(params) [results]
			if interfaceDepth > 0 {
				j := nextNonComment(tokens, i+1)
				if j < len(tokens) && tokens[j].kind == token.LPAREN {
					end := markParamAndResultLists(tokens, mask, j)
					i = end
					continue
				}
			}
		}

		i++
	}

	return mask
}

// markFuncSignature scans tokens starting just after FUNC and marks parameter
// and parenthesized-result type lists. Returns the index to resume scanning.
func markFuncSignature(tokens []tokInfo, mask []bool, start int) int {
	i := nextNonComment(tokens, start)
	if i >= len(tokens) {
		return i
	}

	// Optional receiver: func (r T) Name …
	if tokens[i].kind == token.LPAREN {
		i = markBalancedList(tokens, mask, i)
		i = nextNonComment(tokens, i)
	}

	// Optional name: func Name …  or method name after receiver.
	if i < len(tokens) && tokens[i].kind == token.IDENT {
		i = nextNonComment(tokens, i+1)
	}

	// Parameter list (required for decls; also for function types).
	if i < len(tokens) && tokens[i].kind == token.LPAREN {
		return markParamAndResultLists(tokens, mask, i)
	}
	return i
}

// markParamAndResultLists marks the parameter list at lparenIdx and any
// following parenthesized result list. Returns the index after both.
func markParamAndResultLists(tokens []tokInfo, mask []bool, lparenIdx int) int {
	afterParams := markBalancedList(tokens, mask, lparenIdx)
	j := nextNonComment(tokens, afterParams)
	if j < len(tokens) && tokens[j].kind == token.LPAREN {
		return markBalancedList(tokens, mask, j)
	}
	return afterParams
}

// markBalancedList marks every token from the opening LPAREN through its
// matching RPAREN as inside a type list. Returns the index immediately after
// the closing RPAREN.
func markBalancedList(tokens []tokInfo, mask []bool, lparenIdx int) int {
	depth := 0
	for i := lparenIdx; i < len(tokens); i++ {
		switch tokens[i].kind {
		case token.COMMENT:
			continue
		case token.LPAREN:
			depth++
			mask[i] = true
		case token.RPAREN:
			mask[i] = true
			depth--
			if depth == 0 {
				return i + 1
			}
		default:
			if depth > 0 {
				mask[i] = true
			}
		}
	}
	return len(tokens)
}

func nextNonComment(tokens []tokInfo, i int) int {
	for i < len(tokens) && tokens[i].kind == token.COMMENT {
		i++
	}
	return i
}
