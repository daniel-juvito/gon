// A function literal is checked as a function (v1.13).
//
// Until v1.13 its body was only scanned for calls and composite literals, in the
// scope and with the result contract of the ENCLOSING function. Consequences,
// all measured on v1.12.0: a closure parameter that shadows an outer `!`
// parameter was taken for it (a false GW001), a `return nil` in a closure with a
// `!` result was never reported, and a closure's own `!` parameter was not a
// non-nil name. Found in the audit of the return-operand change (#26).
package checker

import "testing"

const flPre = `package main

type T struct{}
type Box struct{ P !*T }

func Put(p !*T) int { return 0 }

`

func flCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, flPre+body)
}

// --- the two cases of the audit, in return position and not ----------------

func TestFuncLitParameterShadowsAnOuterNonNilParameter(t *testing.T) {
	// in return position
	retWant(t, flCheck(t, `func f(p !*T) func(*T) bool { return func(p *T) bool { return p == nil } }`), 0, 0, 0)
	// not in return position: the same false positive existed there before
	retWant(t, flCheck(t, `func f(p !*T) { g := func(p *T) bool { return p == nil }; _ = g }`), 0, 0, 0)
	// a closure that does NOT shadow still sees the outer `!` parameter
	retWant(t, flCheck(t, `func f(p !*T) func() bool { return func() bool { return p == nil } }`), 0, 0, 1)
}

func TestFuncLitReturnAnswersToItsOwnResultContract(t *testing.T) {
	// the closure's result is `!`: `return nil` is GN001 (in return position)
	retWant(t, flCheck(t, `func f() func() !*T { return func() !*T { return nil } }`), 1, 0, 0)
	// ... and not in return position either
	retWant(t, flCheck(t, `func f() { g := func() !*T { return nil }; _ = g }`), 1, 0, 0)
	// an ordinary closure result is not constrained, even when the enclosing
	// function's result is `!` (the outer contract must not leak in)
	retWant(t, flCheck(t, `func f() !*T { g := func() *T { return nil }; _ = g; return &T{} }`), 0, 0, 0)
	retWant(t, flCheck(t, `func f() func() *T { return func() *T { return nil } }`), 0, 0, 0)
}

func TestFuncLitOwnNonNilParameterIsANonNilName(t *testing.T) {
	retWant(t, flCheck(t, `func f() func(!*T) bool { return func(q !*T) bool { return q == nil } }`), 0, 0, 1)
	retWant(t, flCheck(t, `func f() { g := func(q !*T) bool { return q == nil }; _ = g }`), 0, 0, 1)
}

// --- the enclosing function's context is restored -------------------------

func TestFuncLitRestoresTheEnclosingContext(t *testing.T) {
	// after the closure, `return nil` still answers to the OUTER `!` result
	retWant(t, flCheck(t, `func f() !*T { g := func() *T { return nil }; _ = g; return nil }`), 1, 0, 0)
	// after the closure's parameter scope is gone, the outer `!` name is back
	retWant(t, flCheck(t, `func f(p !*T) bool { g := func(p *T) bool { return p == nil }; _ = g; return p == nil }`), 0, 0, 1)
}

// --- nesting and the call forms -------------------------------------------

func TestFuncLitNested(t *testing.T) {
	retWant(t, flCheck(t, `func f() func() func() !*T { return func() func() !*T { return func() !*T { return nil } } }`), 1, 0, 0)
	// each level has its own result contract
	retWant(t, flCheck(t, `func f() func() *T { return func() *T { g := func() !*T { return nil }; _ = g; return nil } }`), 1, 0, 0)
}

func TestFuncLitDeferGoAndImmediateCall(t *testing.T) {
	retWant(t, flCheck(t, `func f() { defer func() { Put(nil) }() }`), 1, 0, 0)
	retWant(t, flCheck(t, `func f() { go func() { Put(nil) }() }`), 1, 0, 0)
	retWant(t, flCheck(t, `func f() int { return func() int { return Put(nil) }() }`), 1, 0, 0)
}

// --- a closure body is walked as statements -------------------------------

func TestFuncLitBodyIsWalkedAsStatements(t *testing.T) {
	// a local `!` declaration inside a closure
	retWant(t, flCheck(t, `func f() { g := func() { var p !*T = nil; _ = p }; _ = g }`), 1, 0, 0)
	// a local defined from a `!` result is a non-nil source inside the closure
	retWant(t, flCheck(t, `func mk() !*T { return &T{} }
func f() { g := func() bool { x := mk(); return x == nil }; _ = g }`), 0, 0, 1)
	// a clean closure stays clean, reported once
	retWant(t, flCheck(t, `func f() { g := func(p *T) *T { if p == nil { return &T{} }; return p }; _ = g }`), 0, 0, 0)
}

// A violation inside a closure is reported once, not once per enclosing walk.
func TestFuncLitViolationsAreReportedOnce(t *testing.T) {
	retWant(t, flCheck(t, `func f() func() func() Box { return func() func() Box { return func() Box { return Box{P: nil} } } }`), 1, 0, 0)
	retWant(t, flCheck(t, `func f() { g := func() { h := func() { Put(nil) }; h() }; g() }`), 1, 0, 0)
}
