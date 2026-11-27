package checker

import (
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// The plan of gon convert (RFC Part B with gon convert, accepted 2026-11-27): a local that
// Part B would bind `!` is kept nilable, by `x := (T)(e)`, where one of its uses would
// break correct code.

const cvPreamble = `package main

import (
	"bytes"
	bb "bytes"
)

type T struct{ next *T }

func use(p *T)     {}
func ord() *T      { return nil }
func ords() []int  { return nil }
func two() (*T, error) { return nil, nil }
func bang() !*T    { return &T{} }

var _ = bytes.NewReader
var _ = bb.NewReader

`

func cvRun(t *testing.T, body string) (string, ConvertPlan) {
	t.Helper()
	src := cvPreamble + body
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, nil)
	if err != nil {
		t.Fatalf("checker setup: %v", err)
	}
	c.Check()
	plan := c.PlanConvert(result.CleanToOrig)
	return string(ApplyConvert([]byte(src), plan.Edits)), plan
}

// cvWant runs a case and compares the body of the output with the expected body; it
// also checks that a second pass over the output changes nothing (idempotence).
func cvWant(t *testing.T, in, want string) {
	t.Helper()
	out, plan := cvRun(t, in)
	got := strings.TrimPrefix(out, cvPreamble)
	if got != want {
		t.Fatalf("output:\n%s\nwant:\n%s\nplan: %+v", got, want, plan)
	}
	if len(plan.Reports) != 0 {
		t.Fatalf("unexpected reports: %+v", plan.Reports)
	}
	if want != in {
		if _, again := cvRun(t, want); len(again.Edits) != 0 {
			t.Fatalf("not idempotent: a second pass has edits %+v for\n%s", again.Edits, want)
		}
	}
}

type cvCase struct{ name, in, want string }

func TestConvert_RewritesWhereTheBindingWouldBreak(t *testing.T) {
	for _, c := range []cvCase{
		{"nil assignment",
			"func f() { buf := make([]byte, 8); buf = nil; _ = buf }\n",
			"func f() { buf := ([]byte)(make([]byte, 8)); buf = nil; _ = buf }\n"},
		{"an ordinary value",
			"func f() { p := &T{}; p = ord(); _ = p }\n",
			"func f() { p := (*T)(&T{}); p = ord(); _ = p }\n"},
		{"a traversal loop",
			"func f() { for p := new(T); p != nil; p = p.next { use(p) } }\n",
			"func f() { for p := (*T)(new(T)); p != nil; p = p.next { use(p) } }\n"},
		{"its address is taken",
			"func f() { q := new(T); r := &q; _ = r }\n",
			"func f() { q := (*T)(new(T)); r := &q; _ = r }\n"},
		{"a multi-value assignment",
			"func f() { var err error; p := &T{}; p, err = two(); _, _ = p, err }\n",
			"func f() { var err error; p := (*T)(&T{}); p, err = two(); _, _ = p, err }\n"},
		{"a multi-value assignment, the variable second",
			"func rev() (error, *T) { return nil, nil }\nfunc f() { var err error; p := &T{}; err, p = rev(); _, _ = p, err }\n",
			"func rev() (error, *T) { return nil, nil }\nfunc f() { var err error; p := (*T)(&T{}); err, p = rev(); _, _ = p, err }\n"},
		{"a value of another type with the same underlying type",
			"type S []int\nfunc f() { s := []int{1}; s = S{2}; _ = s }\n",
			"type S []int\nfunc f() { s := ([]int)([]int{1}); s = S{2}; _ = s }\n"},
		{"assigned in a closure",
			"func f() { s := []int{1}; g := func() { s = nil }; _, _ = s, g }\n",
			"func f() { s := ([]int)([]int{1}); g := func() { s = nil }; _, _ = s, g }\n"},
		{"range assigns it",
			"func f(ys [][]int) { s := []int{1}; for _, s = range ys {}; _ = s }\n",
			"func f(ys [][]int) { s := ([]int)([]int{1}); for _, s = range ys {}; _ = s }\n"},
		{"a var declaration keeps its form",
			"func f() { var m = map[string]int{}; m = nil; _ = m }\n",
			"func f() { var m = (map[string]int)(map[string]int{}); m = nil; _ = m }\n"},
		{"a type of an imported package, under the file's name for it",
			"func f() { b := &bb.Buffer{}; b = nil; _ = b }\n",
			"func f() { b := (*bb.Buffer)(&bb.Buffer{}); b = nil; _ = b }\n"},
		{"a type parameter",
			"func f[E any]() { x := new(E); x = nil; _ = x }\n",
			"func f[E any]() { x := (*E)(new(E)); x = nil; _ = x }\n"},
		{"a literal that keeps its own !",
			"func f(p !*T) { s := []!*T{p}; s = nil; _ = s }\n",
			"func f(p !*T) { s := ([]*T)([]!*T{p}); s = nil; _ = s }\n"},
		{"only the variable that breaks, of several",
			"func f() { a, b := make([]int, 1), make([]int, 1); b = nil; _, _ = a, b }\n",
			"func f() { a, b := make([]int, 1), ([]int)(make([]int, 1)); b = nil; _, _ = a, b }\n"},
		{"a variable inside the initialiser of another",
			"func f() { g := func() { z := make([]int, 1); z = nil; _ = z }; _ = g }\n",
			"func f() { g := func() { z := ([]int)(make([]int, 1)); z = nil; _ = z }; _ = g }\n"},
	} {
		t.Run(c.name, func(t *testing.T) { cvWant(t, c.in, c.want) })
	}
}

func TestConvert_LeavesTheOthersBound(t *testing.T) {
	for _, c := range []cvCase{
		{"never assigned again", "func f() { p := &T{}; use(p) }\n", ""},
		{"assigned a source", "func f() { s := make([]int, 1); s = make([]int, 2); _ = s }\n", ""},
		{"append, with elements", "func f() { s := []int{1}; s = append(s, 2); _ = s }\n", ""},
		{"append, spread of itself", "func f() { s := []int{1}; s = append(s, ords()...); _ = s }\n", ""},
		{"a slice of itself", "func f() { s := []int{1}; s = s[:0]; _ = s }\n", ""},
		{"a new literal assigned", "func f() { p := &T{}; p = &T{}; _ = p }\n", ""},
		{"an ordinary call, not a candidate", "func f() { p := ord(); p = nil; _ = p }\n", ""},
		{"a result declared !T, bound today", "func f() { p := bang(); _ = p }\n", ""},
		{"not a nilable kind", "func f() { n := 1; n = 2; var a [2]int; _, _ = n, a }\n", ""},
		{"an interface", "func f() { var e error; x := e; x = nil; _ = x }\n", ""},
		{"a variable that exists already", "func f() { p := &T{}; q, p := &T{}, ord(); _, _ = p, q }\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, plan := cvRun(t, c.in)
			if len(plan.Edits) != 0 || len(plan.Reports) != 0 {
				t.Fatalf("expected no edit, got %+v\n%s", plan, strings.TrimPrefix(out, cvPreamble))
			}
		})
	}
}

// A variable initialised from a bound one is bound too, so it is the one that breaks, and
// when the first is given back its nilable form the second is no longer a candidate.
func TestConvert_ChainsOfLocals(t *testing.T) {
	cvWant(t,
		"func f() { x := &T{}; y := x; y = nil; _, _ = x, y }\n",
		"func f() { x := &T{}; y := (*T)(x); y = nil; _, _ = x, y }\n")
	cvWant(t,
		"func f() { x := &T{}; x = nil; y := x; y = nil; _, _ = x, y }\n",
		"func f() { x := (*T)(&T{}); x = nil; y := x; y = nil; _, _ = x, y }\n")
	cvWant(t,
		"func f() { x := &T{}; y := x; x = ord(); _, _ = x, y }\n",
		"func f() { x := (*T)(&T{}); y := x; x = ord(); _, _ = x, y }\n")
}

func TestConvert_ParametersDeclaredBang(t *testing.T) {
	cvWant(t,
		"func f(p !*T) { q := p; q = nil; _ = q }\n",
		"func f(p !*T) { q := (*T)(p); q = nil; _ = q }\n")
	cvWant(t, "func f(p !*T) { q := p; _ = q }\n", "func f(p !*T) { q := p; _ = q }\n")
}

func TestConvert_NoTypeInformationIsAnEmptyPlan(t *testing.T) {
	src := "package main\n\nfunc f() { x := make([]int, 1); x = nil; _ = x }\n"
	result := preproc.Process("t.gon", []byte(src))
	c, err := New("t.gon", result.Clean, result.NonNilOffsets)
	if err != nil {
		t.Fatal(err)
	}
	if plan := c.PlanConvert(result.CleanToOrig); len(plan.Edits) != 0 {
		t.Fatalf("a plan without type information must be empty, got %+v", plan)
	}
}

func TestConvert_ApplyKeepsOuterBeforeInnerAtOneOffset(t *testing.T) {
	out := string(ApplyConvert([]byte("ab"), []ConvertEdit{{1, "["}, {1, "<"}, {2, ">"}, {2, "]"}}))
	if out != "a[<b>]" {
		t.Fatalf("got %q", out)
	}
}
