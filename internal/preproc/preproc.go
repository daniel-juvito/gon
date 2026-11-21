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
	// CleanToOrig maps each byte offset in Clean to the corresponding byte
	// offset in the original Gon source. Used by the Protocol v1 adapter to
	// report positions in Gon space without changing checker internals.
	CleanToOrig []int
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
// A non-nil conversion (!T)(x) also marks a type position: a ! directly after a
// grouping "(" whose closing ")" is followed by "(" (RFC section 6.1). The "("
// groups only when the previous significant token cannot end an operand; after an
// identifier, a literal, ")", "]" or "}" it opens the argument list of a call and
// the source is left unchanged (adder(!flag)(3) keeps its meaning).
//
// A run of ! in a type position is one ! (!!T is !T, RFC section 6.7): every !
// after a type-modifier ! is itself a modifier and records the same clean offset.
//
// Unary ! in expressions (f(a, !b), (!flag), !ok, !!ok) is left untouched.
func Process(filename string, src []byte) *Result {
	fset := token.NewFileSet()
	file := fset.AddFile(filename, 1, len(src))

	var s scanner.Scanner
	// Scanner errors are not reported here — real parse errors are caught later.
	// They are counted, because a conversion is recognised only in a source that
	// scans cleanly (RFC section 6.1, conservative rule).
	scanErrors := 0
	s.Init(file, src, func(pos token.Position, msg string) { scanErrors++ }, scanner.ScanComments)

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

	// inBrack[i] is true when the innermost open bracket enclosing token i is
	// a '['. A ',' there separates type arguments / type parameters (an index
	// expression takes one operand), so a following `!` is a type modifier.
	inBrack := markBracketContext(tokens)

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
					} else if prev.kind == token.COMMA && inBrack[prevIdx] && startsType(tokens, i) {
						// Type-argument list, second or later argument:
						// Box[A, !*T] (v1.8 / M3 — reported as GN003, G4).
						typeModifiers[t.offset] = true
					} else if prev.kind == token.LPAREN && isTypeAssertLParen(tokens, prevIdx) {
						// Type-assertion target: x.(!T). Not a Gon mechanism, but
						// strip the ! and record the offset so the checker can
						// report it cleanly (RFC D6a) instead of a parse error.
						typeModifiers[t.offset] = true
					} else if prev.kind == token.LPAREN && scanErrors == 0 && isNonNilConversion(tokens, i) {
						// Non-nil conversion (!T)(x), RFC section 6.1.
						typeModifiers[t.offset] = true
					}
				case token.NOT:
					// A run of ! in a type position is one !: a ! that follows a
					// type-modifier ! is a modifier too (!!T, (!!T)(x)). After an
					// expression-position ! (!!ok) it is the Go double negation.
					if typeModifiers[prev.offset] {
						typeModifiers[t.offset] = true
					}
				case token.RBRACK:
					// Element position: []!E, [N]!E, [...]!E, map[K]!V (v1.7 /
					// M2b). A ']' is never immediately followed by unary ! in a
					// valid expression (NOT is prefix-only; `a[i]!b` is a syntax
					// error), so this is unambiguously a type modifier.
					typeModifiers[t.offset] = true
				case token.CHAN:
					// chan !E, <-chan !E — channel element position. Stripped so
					// the checker can report GN003 (E11) instead of a parse error.
					typeModifiers[t.offset] = true
				case token.ARROW:
					// chan<- !E: the arrow is a channel direction only when it
					// follows 'chan'. A send statement (`ch <- !flag`) has an
					// expression before the arrow and is left untouched.
					if p := prevNonComment(tokens, prevIdx-1); p >= 0 && tokens[p].kind == token.CHAN {
						typeModifiers[t.offset] = true
					}
				case token.LBRACK:
					// map[!K]V — map key position (checker reports GN003; key
					// contracts are deferred, RFC O1). Guarded by a preceding
					// 'map' keyword so `m[!ok]` indexing is left untouched.
					if p := prevNonComment(tokens, prevIdx-1); p >= 0 && tokens[p].kind == token.MAP {
						typeModifiers[t.offset] = true
					} else if p >= 0 && tokens[p].kind == token.IDENT && startsType(tokens, i) {
						// First type argument: Box[!*T], F[!*T]() (v1.8 / M3 —
						// reported as GN003, G4). Only when the operand begins
						// with a type-only token (`*`, `[`, map, chan, func,
						// interface) so `m[!ok]` indexing is left untouched.
						typeModifiers[t.offset] = true
					}
				case token.TILDE:
					// Constraint term: ~!*T (v1.8 / M3 — reported as GN003, G9).
					// `~` is never an expression operator in Go, so this is
					// unambiguous.
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

	cleanToOrig := make([]int, 0, len(src))
	for origOff := 0; origOff < len(src); origOff++ {
		if typeModifiers[origOff] {
			// This ! is a type modifier. The next character is the start
			// of the type in clean source — record that clean offset.
			nonNilOffsets[len(clean)] = true
			continue // don't copy ! into clean source
		}
		cleanToOrig = append(cleanToOrig, origOff)
		clean = append(clean, src[origOff])
	}

	return &Result{
		Clean:         clean,
		NonNilOffsets: nonNilOffsets,
		CleanToOrig:   cleanToOrig,
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

// prevNonComment returns the index of the nearest non-comment token at or
// before i, or -1 if there is none.
func prevNonComment(tokens []tokInfo, i int) int {
	for i >= 0 && tokens[i].kind == token.COMMENT {
		i--
	}
	return i
}

// isTypeAssertLParen reports whether the LPAREN at lparenIdx opens a type
// assertion — i.e. it is immediately preceded by a PERIOD (`x.(`). Used to
// recognize `x.(!T)` where the `!` is a type modifier, not unary NOT.
func isTypeAssertLParen(tokens []tokInfo, lparenIdx int) bool {
	j := lparenIdx - 1
	for j >= 0 && tokens[j].kind == token.COMMENT {
		j--
	}
	return j >= 0 && tokens[j].kind == token.PERIOD
}

// endsOperand reports whether a token of this kind can end an operand: an
// identifier, an integer, floating-point, imaginary, character or string literal,
// ")", "]" or "}" (RFC section 6.1). A "(" that follows such a token opens the
// argument list of a call; it is not a grouping parenthesis.
func endsOperand(kind token.Token) bool {
	switch kind {
	case token.IDENT, token.INT, token.FLOAT, token.IMAG, token.CHAR, token.STRING,
		token.RPAREN, token.RBRACK, token.RBRACE:
		return true
	}
	return false
}

// isNonNilConversion reports whether the ! at tokens[bangIdx], which follows a
// "(", begins the target type of a non-nil conversion (!T)(x): the "(" directly
// before the !, after skipping the wrapping ")" that close the "(" immediately
// before it, has a closing ")" that is followed by "(", and the outermost of those
// "(" is a grouping parenthesis. The decision is taken on the token stream of the
// Go scanner, which includes the semicolons it inserts at a line end; parentheses
// are balanced with a counter, and the argument list that follows must close.
// Anything else, including an unbalanced source, is not a conversion (RFC
// section 6.1).
func isNonNilConversion(tokens []tokInfo, bangIdx int) bool {
	open := prevNonComment(tokens, bangIdx-1)
	if open < 0 || tokens[open].kind != token.LPAREN {
		return false
	}
	closeIdx := matchingRParen(tokens, open)
	if closeIdx < 0 {
		return false
	}
	for {
		n := nextNonComment(tokens, closeIdx+1)
		if n >= len(tokens) {
			return false
		}
		if tokens[n].kind == token.LPAREN {
			// The argument list of the conversion must close as well.
			if matchingRParen(tokens, n) < 0 {
				return false
			}
			break
		}
		if tokens[n].kind != token.RPAREN {
			return false
		}
		// A wrapping ")": it must close a "(" that directly precedes the
		// outermost "(" seen so far.
		p := prevNonComment(tokens, open-1)
		if p < 0 || tokens[p].kind != token.LPAREN {
			return false
		}
		open = p
		closeIdx = n
	}
	p := prevNonComment(tokens, open-1)
	return p < 0 || !endsOperand(tokens[p].kind)
}

// matchingRParen returns the index of the ")" that closes the "(" at openIdx,
// or -1 when the parentheses do not balance.
func matchingRParen(tokens []tokInfo, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(tokens); i++ {
		switch tokens[i].kind {
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

// startsType reports whether the token after the `!` at tokens[bangIdx] can
// only begin a type, never a boolean operand: `*`, `[`, map, chan, func,
// interface. (`!*p` with p a *bool in an index position is the one theoretical
// ambiguity; it is vanishingly rare and is treated as a type argument.)
func startsType(tokens []tokInfo, bangIdx int) bool {
	j := nextNonComment(tokens, bangIdx+1)
	if j >= len(tokens) {
		return false
	}
	switch tokens[j].kind {
	case token.MUL, token.LBRACK, token.MAP, token.CHAN, token.FUNC, token.INTERFACE:
		return true
	}
	return false
}

// markBracketContext returns, for each token, whether the innermost enclosing
// bracket ((, [, {) is a '['.
func markBracketContext(tokens []tokInfo) []bool {
	mask := make([]bool, len(tokens))
	var stack []token.Token
	for i, t := range tokens {
		if t.kind == token.COMMENT {
			continue
		}
		if n := len(stack); n > 0 && stack[n-1] == token.LBRACK {
			mask[i] = true
		}
		switch t.kind {
		case token.LPAREN, token.LBRACK, token.LBRACE:
			stack = append(stack, t.kind)
		case token.RPAREN, token.RBRACK, token.RBRACE:
			if n := len(stack); n > 0 {
				stack = stack[:n-1]
			}
		}
	}
	return mask
}
