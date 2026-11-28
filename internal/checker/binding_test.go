package checker

import (
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Binding by inference (RFC Part B with gon convert, section 3): a local initialised by
// a form that is certainly non-nil is a `!` variable, unless its address is taken.

const bnPre = `package main

type T struct{ next *T }

func ord() *T { return nil }

`

func bnDiags(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, bnPre+body)
}

func bnMessage(diags []*Diagnostic, code string) string {
	for _, d := range diags {
		if d.Code == code {
			return d.Message
		}
	}
	return ""
}

func TestBinding_TheErrorSaysHowToKeepTheLocalNilable(t *testing.T) {
	diags := bnDiags(t, "func f(q *T) {\n\tx := &T{}\n\tx = nil\n\tx = q\n\t_ = x\n}\n")
	if got := countCode(diags, "GN001"); got != 2 {
		t.Fatalf("want 2 GN001 (nil and an ordinary value): %v", diags)
	}
	for _, d := range diags {
		for _, want := range []string{"x was bound non-nil by its initialiser at 8:2", "var x T = ...", "(T)(e)"} {
			if !strings.Contains(d.Message, want) {
				t.Errorf("the message of %v lacks %q", d, want)
			}
		}
	}
}

func TestBinding_ADeclaredBangHasNoHint(t *testing.T) {
	diags := bnDiags(t, "func f() {\n\tvar x !*T = &T{}\n\tx = nil\n\t_ = x\n}\n")
	if m := bnMessage(diags, "GN001"); m == "" || strings.Contains(m, "bound non-nil by its initialiser") {
		t.Fatalf("a variable declared !T gets the plain message, got %q", m)
	}
}

func TestBinding_WhatIsNotBound(t *testing.T) {
	for name, body := range map[string]string{
		"an ordinary call":        "func f() { x := ord(); x = nil; _ = x }\n",
		"a written type":          "func f() { var x *T = &T{}; x = nil; _ = x }\n",
		"a conversion":            "func f() { x := (*T)(&T{}); x = nil; _ = x }\n",
		"its address is taken":    "func f() { x := &T{}; p := &x; x = nil; _, _ = x, p }\n",
		"taken in a closure":      "func f() { x := &T{}; g := func() *(*T) { return &x }; x = nil; _, _ = x, g }\n",
		"a slice of an ordinary":  "func f(s []int) { x := s[:1]; x = nil; _ = x }\n",
		"a package variable":      "var x = &T{}\n\nfunc f() { x = nil }\n",
		"a parameter":             "func f(x *T) { x = nil }\n",
		"a range variable":        "func f(ts []*T) { for _, x := range ts { x = nil; _ = x } }\n",
		"an interface conversion": "type I interface{ M() }\n\nfunc f(v I) { x := (!I)(v); x = nil; _ = x }\n",
		"a call with two values":  "func two() (*T, error) { return nil, nil }\n\nfunc f() { x, err := two(); x = nil; _, _ = x, err }\n",
	} {
		if diags := bnDiags(t, body); len(diags) != 0 {
			t.Errorf("%s: want no diagnostic, got %v", name, diags)
		}
	}
}

func TestBinding_WhatIsBound(t *testing.T) {
	for name, body := range map[string]string{
		"a composite literal":  "func f() { x := T{}; y := &x; y = nil; _ = y }\n",
		"new":                  "func f() { x := new(T); x = nil; _ = x }\n",
		"make":                 "func f() { x := make(map[string]int); x = nil; _ = x }\n",
		"a function literal":   "func f() { x := func() {}; x = nil; _ = x }\n",
		"append with elements": "func f(s []int) { x := append(s, 1); x = nil; _ = x }\n",
		"an untyped var":       "func f() { var x = &T{}; x = nil; _ = x }\n",
		"a type parameter":     "func f[E any]() { x := new(E); x = nil; _ = x }\n",
		"a loop variable":      "func f() { for p := new(T); p != nil; p = nil { } }\n",
		"a bound local":        "func f() { a := &T{}; b := a; b = nil; _ = b }\n",
		"per position":         "func f() { a, b := &T{}, ord(); a = nil; b = nil; _, _ = a, b }\n",
	} {
		if diags := bnDiags(t, body); countCode(diags, "GN001") == 0 {
			t.Errorf("%s: want a GN001, got %v", name, diags)
		}
	}
}

func TestBinding_ACallWithABangResultKeepsItsOwnRule(t *testing.T) {
	// Bound by return-value contracts since v1.1, not by inference: no hint is added.
	diags := bnDiags(t, "func nn() !*T { return &T{} }\n\nfunc f() { x := nn(); x = nil; _ = x }\n")
	if m := bnMessage(diags, "GN001"); m == "" || strings.Contains(m, "bound non-nil by its initialiser") {
		t.Fatalf("a !T call binds as before, with the plain message, got %q (%v)", m, diags)
	}
}

func TestBinding_AppendOfASpreadKeepsTheRecognitionOfItsBase(t *testing.T) {
	bound := bnDiags(t, "func f(s []int) { a := []int{1}; x := append(a, s...); x = nil; _ = x }\n")
	if countCode(bound, "GN001") != 1 {
		t.Fatalf("append(bound, s...) is bound: %v", bound)
	}
	ordinary := bnDiags(t, "func f(a, s []int) { x := append(a, s...); x = nil; _ = x }\n")
	if len(ordinary) != 0 {
		t.Fatalf("append(ordinary, s...) binds nothing: %v", ordinary)
	}
}

func TestBinding_AComparisonWithNilIsAWarning(t *testing.T) {
	diags := bnDiags(t, "func f() { x := &T{}; if x == nil { } }\n")
	if countCode(diags, "GW001") != 1 || countCode(diags, "GN001") != 0 {
		t.Fatalf("x == nil on a bound local is GW001: %v", diags)
	}
}

func TestBinding_AFlowIntoABangSlotNeedsNoConversion(t *testing.T) {
	diags := bnDiags(t, "func use(p !*T) {}\n\nfunc f(q *T) {\n\tx := &T{}\n\tuse(x)\n\ty := q\n\tuse(y)\n}\n")
	if countCode(diags, "GN001") != 1 {
		t.Fatalf("use(x) is silent (x is bound), use(y) is GN001 (y is ordinary): %v", diags)
	}
}

func TestBinding_NoTypeInformationBindsNothing(t *testing.T) {
	src := bnPre + "func f() { x := &T{}; x = nil; _ = x }\n"
	result := preproc.Process("t.gon", []byte(src))
	c, err := New("t.gon", result.Clean, result.NonNilOffsets)
	if err != nil {
		t.Fatal(err)
	}
	if diags := c.Check(); countCode(diags, "GN001") != 0 {
		t.Fatalf("without type information nothing is bound: %v", diags)
	}
}

func TestBinding_AConvertedLocalIsNotBound(t *testing.T) {
	// What gon convert writes: the rewrite of every site that the binding would break
	// leaves no diagnostic of the binding.
	diags := bnDiags(t, "func f(q *T) {\n\tx := (*T)(&T{})\n\tx = nil\n\tx = q\n\tif x == nil {\n\t}\n\t_ = x\n}\n")
	if len(diags) != 0 {
		t.Fatalf("a local kept nilable by (T)(e) has no diagnostic: %v", diags)
	}
}
