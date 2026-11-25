package checker

import (
	"strings"
	"testing"
)

// Rules A, B and C1 of the RFC on local backdoors (accepted 2026-11-25): a
// zero-valued array of `!` elements and a `make` of a slice of `!` elements are
// GN002, and a named `!T` result is a `!` variable in the body of a declaration.

const zvPreamble = `package main

type T struct{}
type Slots [4]!*T
type Ptrs []!*T
type Holder struct{ A [2]!*T }

func ord() *T { return nil }
func nn() !*T { return &T{} }

const zero = 0

`

func zvCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, zvPreamble+body)
}

func zvWant(t *testing.T, d []*Diagnostic, code string, n int) {
	t.Helper()
	if got := countCode(d, code); got != n {
		t.Fatalf("expected %d %s, got %d: %v", n, code, got, d)
	}
}

func TestZV_A_ZeroValuedArraySites(t *testing.T) {
	for name, body := range map[string]string{
		"var":         `func f() { var a [3]!*T; _ = a }`,
		"var package": `var g [3]!*T`,
		"new":         `func f() { _ = new([3]!*T) }`,
		"named type":  `func f() { var s Slots; _ = s }`,
		"named new":   `func f() { _ = new(Slots) }`,
		"struct var":  `func f() { var h Holder; _ = h }`,
		"struct lit":  `func f() { _ = Holder{} }`,
		"struct new":  `func f() { _ = new(Holder) }`,
		"map of":      `func f() { var a [2]![]T; _ = a }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := zvCheck(t, body)
			zvWant(t, d, "GN002", 1)
			if !strings.Contains(d[0].Message, "zero-valued array") {
				t.Errorf("message = %q", d[0].Message)
			}
		})
	}
}

func TestZV_A_SilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"zero length":      `func f() { var a [0]!*T; _ = a }`,
		"full literal":     `func f() { _ = [2]!*T{nn(), nn()} }`,
		"initialiser":      `func f() { var a [2]!*T = [2]!*T{nn(), nn()}; _ = a }`,
		"plain pointers":   `func f() { var a [3]*T; _ = a }`,
		"no element bang":  `func f() { var a [3]int; _ = a }`,
		"slice":            `func f() { var s []!*T; _ = s }`,
		"inferred length":  `func f() { _ = [...]!*T{nn(), nn()} }`,
		"pointer to array": `func f() { var p *[3]!*T; _ = p }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 0)
		})
	}
}

func TestZV_A_StructFieldNamesTheField(t *testing.T) {
	d := zvCheck(t, `func f() { var h Holder; _ = h }`)
	zvWant(t, d, "GN002", 1)
	if !strings.Contains(d[0].Message, "field A") {
		t.Errorf("message = %q, want the field named", d[0].Message)
	}
}

func TestZV_B_MakeSlice(t *testing.T) {
	for name, body := range map[string]string{
		"len 3":         `func f() { _ = make([]!*T, 3) }`,
		"variable":      `func f(n int) { _ = make([]!*T, n) }`,
		"with cap":      `func f() { _ = make([]!*T, 2, 8) }`,
		"named slice":   `func f(n int) { _ = make(Ptrs, n) }`,
		"const nonzero": `func f() { const k = 2; _ = make([]!*T, k) }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 1)
		})
	}
}

func TestZV_B_SilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"len 0":         `func f() { _ = make([]!*T, 0) }`,
		"len 0 cap n":   `func f(n int) { _ = make([]!*T, 0, n) }`,
		"const zero":    `func f() { _ = make([]!*T, zero) }`,
		"plain slice":   `func f(n int) { _ = make([]*T, n) }`,
		"map":           `func f(n int) { _ = make(map[string]!*T, n) }`,
		"channel":       `func f(n int) { _ = make(chan *T, n) }`,
		"user function": `func make(a []int, n int) {}; func g() { make(nil, 3) }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 0)
		})
	}
}

// The known limit that rule B does not close: a capacity makes elements that a
// reslice exposes. The rule looks at the length only.
func TestZV_B_ResliceOverCapacityStaysAccepted(t *testing.T) {
	zvWant(t, zvCheck(t, `func f() { s := make([]!*T, 0, 3); s = s[:3]; _ = s }`), "GN002", 0)
}

func TestZV_C1_NamedResultIsABangVariable(t *testing.T) {
	d := zvCheck(t, `func f() (x !*T) { x = nil; return nn() }`)
	zvWant(t, d, "GN001", 1)
	d = zvCheck(t, `func f() (x !*T) { x = ord(); return nn() }`)
	zvWant(t, d, "GN001", 1)
	d = zvCheck(t, `func f() (x !*T) { x = nn(); if x == nil {}; return nn() }`)
	zvWant(t, d, "GW001", 1)
	zvWant(t, d, "GN001", 0)
}

func TestZV_C1_NamedResultSilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"plain result":    `func f() (x *T) { x = nil; return x }`,
		"mixed":           `func f() (x !*T, y *T) { x = nn(); y = nil; return x, y }`,
		"source assigned": `func f() (x !*T) { x = &T{}; return x }`,
		"unnamed":         `func f() !*T { return nn() }`,
		"method":          `type R struct{}; func (R) m() (x *T) { x = nil; return }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := zvCheck(t, body)
			zvWant(t, d, "GN001", 0)
			zvWant(t, d, "GW001", 0)
		})
	}
}

func TestZV_C1_MethodNamedBangResult(t *testing.T) {
	d := zvCheck(t, `type R struct{}; func (R) m() (x !*T) { x = nil; return nn() }`)
	zvWant(t, d, "GN001", 1)
}
