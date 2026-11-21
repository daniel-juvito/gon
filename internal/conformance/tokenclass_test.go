package conformance

import (
	"go/scanner"
	"go/token"
	"strings"
	"testing"
)

// The token-class table (SF-1 of the plan): for the first "(" that is followed
// by "!", the class of the previous significant token decides whether that "("
// is a grouping parenthesis (RFC section 6.1). The rows are written by hand;
// the expected previous token is checked against the Go scanner, including the
// semicolons it inserts at a line end, and the class is derived from the list
// of RFC section 6.1: the tokens that can end an operand are an identifier, a
// literal, ")", "]" and "}".
type tokenClassRow struct {
	id       string
	src      string
	prev     string // the previous significant token, as token.Token.String()
	grouping bool
}

var tokenClassRows = []tokenClassRow{
	{"TC-01", "v := (!*N)(x)", ":=", true},
	{"TC-02", "v = (!*N)(x)", "=", true},
	{"TC-03", "func f() *N { return (!*N)(x) }", "return", true},
	{"TC-04", "f(a, (!*N)(x))", ",", true},
	{"TC-05", "f((!*N)(x))", "(", true},
	{"TC-06", "s := []*N{(!*N)(x)}", "{", true},
	{"TC-07", "switch k {\ncase 1:\n\t(!*N)(x).Visit()\n}", ":", true},
	{"TC-08", "ok := a && (!*N)(x) != nil", "&&", true},
	{"TC-09", "n := 1 + (!*N)(x).V", "+", true},
	{"TC-10", "ch <- (!*N)(x)", "<-", true},
	{"TC-11", "a; (!*N)(x).Visit()", ";", true},
	{"TC-12", "if ok {\n}\n(!*N)(x).Visit()", ";", true},
	{"TC-13", "f()\n(!*N)(x).Visit()", ";", true},
	{"TC-14", "defer (!*N)(x).Close()", "defer", true},
	{"TC-15", "v := /* c */ (!*N)(x)", ":=", true},
	{"TC-16", "w := adder(!flag)(3)", "IDENT", false},
	{"TC-17", "w := 1(!a)(b)", "INT", false},
	{"TC-18", "w := 1.5(!a)(b)", "FLOAT", false},
	{"TC-19", "w := 2i(!a)(b)", "IMAG", false},
	{"TC-20", "w := 'a'(!a)(b)", "CHAR", false},
	{"TC-21", "w := \"s\"(!a)(b)", "STRING", false},
	{"TC-22", "w := f(x)(!a)(b)", ")", false},
	{"TC-23", "w := m[k](!a)(b)", "]", false},
	{"TC-24", "w := func() {}(!a)(b)", "}", false},
	{"TC-25", "w := adder /* c */ (!flag)(3)", "IDENT", false},
	{"TC-26", "w := p.f(!a)(b)", "IDENT", false},
}

// endsOperand is the list of RFC section 6.1.
func endsOperand(k token.Token) bool {
	switch k {
	case token.IDENT, token.INT, token.FLOAT, token.IMAG, token.CHAR, token.STRING,
		token.RPAREN, token.RBRACK, token.RBRACE:
		return true
	}
	return false
}

func TestTokenClassTable(t *testing.T) {
	for _, row := range tokenClassRows {
		t.Run(row.id, func(t *testing.T) {
			src := []byte(row.src)
			fset := token.NewFileSet()
			f := fset.AddFile("", fset.Base(), len(src))
			var s scanner.Scanner
			s.Init(f, src, nil, scanner.ScanComments) // as the preprocessor does

			var toks []token.Token
			for {
				_, k, _ := s.Scan()
				if k == token.EOF {
					break
				}
				toks = append(toks, k)
			}
			prevSig := func(i int) token.Token {
				for j := i - 1; j >= 0; j-- {
					if toks[j] != token.COMMENT {
						return toks[j]
					}
				}
				return token.ILLEGAL
			}
			nextSig := func(i int) token.Token {
				for j := i + 1; j < len(toks); j++ {
					if toks[j] != token.COMMENT {
						return toks[j]
					}
				}
				return token.EOF
			}
			idx := -1
			for i, k := range toks {
				if k == token.LPAREN && nextSig(i) == token.NOT {
					idx = i
					break
				}
			}
			if idx < 0 {
				t.Fatalf("no \"(\" followed by \"!\" in %q", row.src)
			}
			prev := prevSig(idx)
			if got := strings.TrimSpace(prev.String()); got != row.prev {
				t.Fatalf("previous significant token = %q, the table says %q (source %q)", got, row.prev, row.src)
			}
			if grouping := !endsOperand(prev); grouping != row.grouping {
				t.Errorf("grouping = %v for previous token %s, the table says %v", grouping, prev, row.grouping)
			}
		})
	}
}

// TestTokenClassIDsMatchManifest keeps the table and the manifest in step.
func TestTokenClassIDsMatchManifest(t *testing.T) {
	inTable := map[string]bool{}
	for _, r := range tokenClassRows {
		inTable[r.id] = true
	}
	inManifest := map[string]bool{}
	for _, e := range loadManifest(t) {
		if e.Kind == "tokenclass" {
			inManifest[e.ID] = true
		}
	}
	for id := range inTable {
		if !inManifest[id] {
			t.Errorf("table row %s is not in the manifest", id)
		}
	}
	for id := range inManifest {
		if !inTable[id] {
			t.Errorf("manifest entry %s has no table row", id)
		}
	}
}
