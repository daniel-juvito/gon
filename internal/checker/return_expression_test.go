// The operands of a `return` are checked like every other expression (v1.13).
//
// Until v1.13 only the v1.7 element contracts ran on them: `_ = Box{P: nil}` was
// GN001 while `return Box{P: nil}` was silently accepted, as were a nil passed to
// a `!` parameter, a missing required field, `new(T)`, and a comparison of a
// non-nil name with nil. That was an oversight of coverage, not a design
// exception: every other statement form was already walked.
package checker

import "testing"

const retPre = `package main

type T struct{}
type Box struct{ P !*T }
type Row struct{ Cells []!*T }
type Outer struct{ In Box }
type Svc struct{}

func Put(p !*T) int { return 0 }

func (s *Svc) Do(p !*T) int { return 0 }

`

func retCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, retPre+body)
}

func retWant(t *testing.T, diags []*Diagnostic, gn001, gn002, gw001 int) {
	t.Helper()
	got := [3]int{countCode(diags, "GN001"), countCode(diags, "GN002"), countCode(diags, "GW001")}
	if got != [3]int{gn001, gn002, gw001} || len(diags) != gn001+gn002+gw001 {
		t.Fatalf("GN001/GN002/GW001 = %v, want %v: %v", got, [3]int{gn001, gn002, gw001}, diags)
	}
}

func TestReturnCompositeLiteralFieldNil(t *testing.T) {
	retWant(t, retCheck(t, `func f() Box { return Box{P: nil} }`), 1, 0, 0)
}

func TestReturnCompositeLiteralMissingRequiredField(t *testing.T) {
	retWant(t, retCheck(t, `func f() Box { return Box{} }`), 0, 1, 0)
}

func TestReturnNewOfAStructWithARequiredField(t *testing.T) {
	retWant(t, retCheck(t, `func f() *Box { return new(Box) }`), 0, 1, 0)
}

func TestReturnNilArgumentToANonNilParameter(t *testing.T) {
	retWant(t, retCheck(t, `func f() int { return Put(nil) }`), 1, 0, 0)
}

func TestReturnComparisonOfANonNilNameWithNil(t *testing.T) {
	retWant(t, retCheck(t, `func f(p !*T) bool { return p == nil }`), 0, 0, 1)
	retWant(t, retCheck(t, `func f(p !*T) bool { return p != nil }`), 0, 0, 1)
}

// v1.11 method contracts apply in return position too.
func TestReturnMethodCallNilArgument(t *testing.T) {
	retWant(t, retCheck(t, `func f(s *Svc) int { return s.Do(nil) }`), 1, 0, 0)
}

// A violation nested inside a larger expression is found.
func TestReturnNestedViolations(t *testing.T) {
	retWant(t, retCheck(t, `func f() Outer { return Outer{In: Box{P: nil}} }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f() int { return 1 + Put(nil) }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f(m map[string]Box) Box { return m["k"] }`), 0, 0, 0)
}

// Each operand of a multi-value return is checked.
func TestReturnMultipleOperands(t *testing.T) {
	retWant(t, retCheck(t, `func f() (int, Box, bool) { return Put(nil), Box{P: nil}, Put(nil) > 0 }`), 3, 0, 0)
	retWant(t, retCheck(t, `func f(p !*T) (int, Box, bool) { return Put(nil), Box{}, p == nil }`), 1, 1, 1)
}

// v1.7 element contracts still fire in return position, exactly once.
func TestReturnElementContractsAreNotReportedTwice(t *testing.T) {
	retWant(t, retCheck(t, `func f() []!*T { return []!*T{nil} }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f() Row { return Row{Cells: []!*T{nil}} }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f() map[string]!*T { return map[string]!*T{"a": nil} }`), 1, 0, 0)
}

// The existing return-site rules are unchanged and not doubled: `return nil` for
// a `!` result is one GN001, and a clean return is clean.
func TestReturnExistingRulesAreUnchanged(t *testing.T) {
	retWant(t, retCheck(t, `func f() !*T { return nil }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f() !*T { return &T{} }`), 0, 0, 0)
	retWant(t, retCheck(t, `func f() Box { return Box{P: &T{}} }`), 0, 0, 0)
	retWant(t, retCheck(t, `func f() { return }`), 0, 0, 0)
	retWant(t, retCheck(t, `func f() (b Box) { b.P = &T{}; return }`), 0, 0, 0)
}

// Function literals: one in the operand, and a return inside one.
func TestReturnFunctionLiterals(t *testing.T) {
	// the literal's body is part of the operand
	retWant(t, retCheck(t, `func f() func() { return func() { Put(nil) } }`), 1, 0, 0)
	// a return inside a literal is a return like any other
	retWant(t, retCheck(t, `func f() func() Box { return func() Box { return Box{P: nil} } }`), 1, 0, 0)
	// reported once, not once per enclosing return
	retWant(t, retCheck(t, `func f() func() func() int { return func() func() int { return func() int { return Put(nil) } } }`), 1, 0, 0)
}

// Other statement forms were already walked and stay so (no double reporting).
func TestReturnDoesNotChangeOtherStatementForms(t *testing.T) {
	retWant(t, retCheck(t, `func f() { _ = Box{P: nil}; Put(nil); x := Put(nil); _ = x }`), 3, 0, 0)
}

// Without type information the contracts that need it stay inert, but the
// syntactic ones still apply in return position.
func TestReturnWithoutTypeInformation(t *testing.T) {
	d := checkSource(t, retPre+`func f() Box { return Box{P: nil} }`)
	if countCode(d, "GN001") != 1 {
		t.Fatalf("a composite literal with a nil field is syntactic: %v", d)
	}
}

// A type assertion in return position: `x.(!T)` is rejected (v1.6), as it is in
// every other position.
func TestReturnTypeAssertion(t *testing.T) {
	retWant(t, retCheck(t, `func f(v any) *T { return v.(!*T) }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f(v any) *T { p := v.(!*T); return p }`), 1, 0, 0)
	retWant(t, retCheck(t, `func f(v any) *T { return v.(*T) }`), 0, 0, 0)
}
