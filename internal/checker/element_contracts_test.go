// Element-contract construction (v1.7 / M2b).
//
// Executable specification for docs/rfc-element-contract-construction.md §7.
// An element `!` (`[]!*T`, `map[K]!V`, `[N]!*T`) is a construction-time
// obligation: every explicitly written element of a composite literal, and
// every zero-filled index of a fixed array, must be non-nil at the site.
// It is not a lifetime invariant of the container.
//
// Observable discriminants:
//
//	GN001 — explicit nil written as an element / map value under an element `!`
//	GN002 — fixed-array literal leaves a `!` nilable element index at zero
//	GN003 — element `!` on a channel element, map key, or non-nilable kind
package checker

import "testing"

const elemPreamble = `package main
type T struct{}
type I interface{ M() }
func mk() *T { return &T{} }
func maybe() *T { return nil }
`

func elemCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, elemPreamble+body)
}

// --- E4: explicit nil element -> GN001 -------------------------------------

func TestEC_SliceElementNil(t *testing.T) {
	d := elemCheck(t, `func f() []!*T { return []!*T{nil} }`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("nil slice element must be GN001: %v", d)
	}
}

func TestEC_SliceElementsOK(t *testing.T) {
	d := elemCheck(t, `func f() { _ = []!*T{mk(), mk()} }`)
	if len(d) != 0 {
		t.Fatalf("non-nil slice elements must be clean: %v", d)
	}
}

func TestEC_EmptySliceOK(t *testing.T) {
	d := elemCheck(t, `func f() { _ = []!*T{} }`)
	if len(d) != 0 {
		t.Fatalf("empty element slice must be clean: %v", d)
	}
}

func TestEC_OrdinaryCallElementAccepted(t *testing.T) {
	// E4: only the literal nil is rejected; an ordinary expression that may
	// return nil is accepted (conservative, Type Coverage C4).
	d := elemCheck(t, `func f() { _ = []!*T{maybe()} }`)
	if countCode(d, "GN001") != 0 {
		t.Fatalf("ordinary call element must be accepted: %v", d)
	}
}

func TestEC_MapValueNil(t *testing.T) {
	d := elemCheck(t, `func f() {
	_ = map[string]!*T{"a": mk(), "b": nil}
}`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("nil map value must be GN001: %v", d)
	}
}

func TestEC_MapValuesOK(t *testing.T) {
	d := elemCheck(t, `func f() { _ = map[string]!*T{"a": mk()} }`)
	if len(d) != 0 {
		t.Fatalf("non-nil map values must be clean: %v", d)
	}
}

// --- E5: fixed-array zero-fill -> one GN002 -------------------------------

func TestEC_FixedArrayShortfall(t *testing.T) {
	cases := map[string]string{
		"unkeyed":    `var a [3]!*T = [3]!*T{mk()}`,
		"keyed_high": `var b [3]!*T = [3]!*T{2: mk()}`,
		"keyed_gap":  `var c [3]!*T = [3]!*T{0: mk(), 2: mk()}`,
		"empty":      `var e [3]!*T = [3]!*T{}`,
	}
	for name, decl := range cases {
		t.Run(name, func(t *testing.T) {
			d := elemCheck(t, "func f() {\n\t"+decl+"\n\t_ = a\n}")
			if got := countCode(d, "GN002"); got != 1 {
				t.Fatalf("%s: want exactly 1 GN002, got %d: %v", name, got, d)
			}
		})
	}
}

func TestEC_FixedArrayFullyCoveredOK(t *testing.T) {
	cases := map[string]string{
		"unkeyed": `var d [3]!*T = [3]!*T{mk(), mk(), mk()}`,
		"keyed":   `var d [3]!*T = [3]!*T{0: mk(), 1: mk(), 2: mk()}`,
		"exact":   `var d [2]!*T = [2]!*T{mk(), mk()}`,
		"zerolen": `var d [0]!*T = [0]!*T{}`,
	}
	for name, decl := range cases {
		t.Run(name, func(t *testing.T) {
			d := elemCheck(t, "func f() {\n\t"+decl+"\n\t_ = d\n}")
			if got := countCode(d, "GN002"); got != 0 {
				t.Fatalf("%s: want 0 GN002, got %d: %v", name, got, d)
			}
		})
	}
}

func TestEC_InferredLengthArrayNoShortfall(t *testing.T) {
	d := elemCheck(t, `func f() { _ = [...]!*T{mk()} }`)
	if countCode(d, "GN002") != 0 {
		t.Fatalf("[...]!*T has no shortfall: %v", d)
	}
}

// --- E6: interaction with outer reference contracts ----------------------

func TestEC_OuterAndElementComposition(t *testing.T) {
	d := elemCheck(t, `func f() {
	var s ![]!*T = nil
	var u ![]!*T = []!*T{}
	var v ![]!*T = []!*T{nil}
	_, _, _ = s, u, v
}`)
	// s: outer nil (GN001); v: element nil (GN001); u: clean.
	if got := countCode(d, "GN001"); got != 2 {
		t.Fatalf("want 2 GN001 (outer + element), got %d: %v", got, d)
	}
}

func TestEC_OuterZeroValueStillGN002(t *testing.T) {
	d := elemCheck(t, `func f() {
	var u ![]!*T
	_ = u
}`)
	if countCode(d, "GN002") != 1 {
		t.Fatalf("bare ![]!*T var is still GN002 (v1.6): %v", d)
	}
}

// --- E12: element interface contracts -----------------------------------

func TestEC_InterfaceElementNil(t *testing.T) {
	d := elemCheck(t, `func f() { _ = []!I{nil} }`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("nil interface element must be GN001: %v", d)
	}
}

func TestEC_InterfaceElementOK(t *testing.T) {
	d := elemCheck(t, `func f(x I) { _ = []!I{x} }`)
	if countCode(d, "GN001") != 0 {
		t.Fatalf("ordinary interface element must be accepted: %v", d)
	}
}

// --- E7 / non-goals: lifetime is not tracked ---------------------------

func TestEC_NonGoalsStaySilent(t *testing.T) {
	d := elemCheck(t, `func f() {
	s := []!*T{mk()}
	s[0] = nil
	s = append(s, nil)
	for _, p := range s {
		_ = p
	}
	_ = s
}`)
	if len(d) != 0 {
		t.Fatalf("post-construction mutation must not be diagnosed: %v", d)
	}
}

// --- E11 / O1 / O4: malformed element ! -> GN003 ----------------------

func TestEC_MalformedElementBang(t *testing.T) {
	cases := map[string]string{
		"chan":     `func f() { var c chan !*T; _ = c }`,
		"chandir":  `func f() { var c chan<- !*T; _ = c }`,
		"recvchan": `func f() { var c <-chan !*T; _ = c }`,
		"sliceint": `func f() { var s []!int; _ = s }`,
		"mapkey":   `func f() { var m map[!*T]int; _ = m }`,
		"arrayint": `type Xs [2]!int`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			d := elemCheck(t, body)
			if got := countCode(d, "GN003"); got != 1 {
				t.Fatalf("%s: want exactly 1 GN003, got %d: %v", name, got, d)
			}
		})
	}
}

// --- E3: nested / elided literals -------------------------------------

func TestEC_NestedElidedLiteral(t *testing.T) {
	d := elemCheck(t, `func f() { _ = [][]!*T{{nil}} }`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("nil in elided inner literal must be GN001: %v", d)
	}
}

func TestEC_NestedElidedFixedArrayShortfall(t *testing.T) {
	// Each elided inner [2]!*T literal is a fixed-array construction site.
	d := elemCheck(t, `func f() { _ = [2][2]!*T{{mk()}, {mk()}} }`)
	if countCode(d, "GN002") != 2 {
		t.Fatalf("want 2 GN002 (one per inner literal), got %d: %v", countCode(d, "GN002"), d)
	}
}

// --- construction sites: return position -----------------------------

func TestEC_ReturnSiteElement(t *testing.T) {
	d := elemCheck(t, `func f() []!*T { return []!*T{nil} }`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("nil element in return literal must be GN001: %v", d)
	}
}
