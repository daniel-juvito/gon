// Interface non-nil contracts (v1.4 / M1a).
//
// Executable specification for docs/rfc-interface-semantics.md §8. `!I` means
// only that the interface value is non-nil; it never constrains the dynamic
// value. The single new rule relative to concrete `!T` is D3b: an ordinary
// interface-typed expression cannot satisfy `!I`, unconditionally.
package checker

import (
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// checkTyped runs the checker with type information available (via the
// annotations path) but no external .gna resolver.
func checkTyped(t *testing.T, src string) []*Diagnostic {
	t.Helper()
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, nil)
	if err != nil {
		t.Fatalf("checker setup: %v", err)
	}
	return c.Check()
}

func mustGN001(t *testing.T, diags []*Diagnostic, n int) {
	t.Helper()
	if got := countCode(diags, "GN001"); got != n {
		t.Fatalf("expected %d GN001, got %d: %v", n, got, diags)
	}
}

func mustNoGN001Iface(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if got := countCode(diags, "GN001"); got != 0 {
		t.Fatalf("expected no GN001, got %d: %v", got, diags)
	}
}

// §8: Concrete (including typed-nil) → !I accepted.
func TestIface_ConcreteToBangAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (T) M() {}
func f() {
	var p *T
	var r !R = p
	_ = r
}`)
	mustNoGN001Iface(t, diags)
}

// §4.2 / §3.10: a typed-nil dynamic value never violates !I.
func TestIface_TypedNilConcreteToBangAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (t *T) M() {}
func f() {
	var p *T = nil
	var r !R = p
	_ = r
}`)
	if len(diags) != 0 {
		t.Fatalf("typed-nil concrete → !I must be silent: %v", diags)
	}
}

// §8 / D3b / §3.3: ordinary I → !I rejected (GN001).
func TestIface_OrdinaryToBangRejected(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func src() R { return nil }
func f() {
	var r R = src()
	var x !R = r
	_ = x
}`)
	mustGN001(t, diags, 1)
}

// §3.3 / §4.5: the rejection stands even after an explicit nil check —
// flow-sensitive narrowing is outside the semantics of !I.
func TestIface_OrdinaryToBangRejectedAfterNilCheck(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func src() R { return nil }
func f() {
	var r R = src()
	if r != nil {
		var x !R = r
		_ = x
	}
}`)
	mustGN001(t, diags, 1)
}

// §8 / D3c / §3.4: nil literal → !I rejected (GN001).
func TestIface_NilLiteralToBangRejected(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func f() {
	var r !R = nil
	_ = r
}`)
	mustGN001(t, diags, 1)
}

// §8 / D3d / §3.5: !I → !I accepted.
func TestIface_BangToBangAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (T) M() {}
func mk() !R { var v T; return v }
func f() {
	var r !R = mk()
	var x !R = r
	_ = x
}`)
	mustNoGN001Iface(t, diags)
}

// §8 / §3.6 / D4: a result declared !I is a non-nil source at the call site.
func TestIface_ReturnBangIsNonNilSource(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (T) M() {}
func mk() !R { var v T; return v }
func f() {
	r := mk()
	if r == nil {
	}
}`)
	if countCode(diags, "GW001") != 1 {
		t.Fatalf("expected GW001 on nil comparison of non-nil source, got: %v", diags)
	}
	mustNoGN001Iface(t, diags)
}

// §4.7: a !I function may return a typed-nil concrete dynamic value.
func TestIface_ReturnTypedNilConcreteAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (t *T) M() {}
func mk() !R {
	var p *T = nil
	return p
}`)
	if len(diags) != 0 {
		t.Fatalf("returning typed-nil concrete from !I must be silent: %v", diags)
	}
}

// §3.6 / D4: returning an ordinary interface value from a !I result is rejected.
func TestIface_ReturnOrdinaryRejected(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func passthru(r R) !R {
	return r
}`)
	mustGN001(t, diags, 1)
}

// §8 / §3.9 / D6c / §4.9a: explicit conversion I(concrete) yields ordinary I
// and cannot satisfy !I, even though the operand is concrete.
func TestIface_ExplicitConversionCannotSatisfyBang(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (t *T) M() {}
func f() {
	var p *T = nil
	var good !R = p
	var bad !R = R(p)
	_ = good
	_ = bad
}`)
	mustGN001(t, diags, 1) // only the R(p) line
}

// §8 / §3.8 / D6a: a type-assertion target cannot be !I.
func TestIface_AssertionTargetBangRejected(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func f(x any) {
	r := x.(!R)
	_ = r
}`)
	mustGN001(t, diags, 1)
}

func TestIface_AssertionTargetBangCommaOkRejected(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func f(x any) {
	r, ok := x.(!R)
	_ = r
	_ = ok
}`)
	mustGN001(t, diags, 1)
}

// §8 / §3.7 / §4.10: interface embedding does not propagate `!`.
func TestIface_EmbeddingDoesNotPropagate(t *testing.T) {
	diags := checkTyped(t, `package main
type Reader interface{ Read() }
type ReadCloser interface {
	Reader
	Close()
}
func f(rc !ReadCloser, r Reader) {
	if rc == nil {
	}
	if r == nil {
	}
}`)
	// Only rc carries a contract; r stays ordinary. Exactly one GW001.
	if got := countCode(diags, "GW001"); got != 1 {
		t.Fatalf("embedding must not propagate ! to Reader: want 1 GW001, got %d: %v", got, diags)
	}
}

// §8: dynamic-value nilness is never reported as a violation of !I.
func TestIface_DynamicValueNilnessNeverReported(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (t *T) M() {}
func mk() !R {
	var p *T
	return p
}
func f() {
	r := mk()
	_ = r
}`)
	if len(diags) != 0 {
		t.Fatalf("dynamic-value nilness must never be flagged: %v", diags)
	}
}

// §8: passing an ordinary interface value to a !I parameter is rejected;
// a concrete operand (D3a) and an existing !I source (D3d) are accepted.
func TestIface_ParamContract(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
type T struct{}
func (t *T) M() {}
func want(r !R) {}
func f(ord R) {
	var p *T
	want(p)   // D3a: concrete → ok
	want(ord) // D3b: ordinary interface → GN001
}`)
	mustGN001(t, diags, 1)
}

// §6: a program that does not use interface contracts is unchanged — an
// ordinary interface value may still be compared, assigned, and passed.
func TestIface_NoContractProgramUnchanged(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func src() R { return nil }
func sink(r R) {}
func f() {
	var r R = src()
	if r == nil {
	}
	sink(r)
	var x R = r
	_ = x
}`)
	if len(diags) != 0 {
		t.Fatalf("ordinary interface code must be untouched: %v", diags)
	}
}

// D7 / D4: a `.gna`-declared `!I` result is a non-nil source at the call site,
// using the same Type.NonNil representation as concrete `!T`.
func TestIface_GNAResultBangIsNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Reader:
    results:
      - "!R"
`
	gonSrc := `package main
import "demo"
type R interface{ M() }
func f() {
	r := demo.Reader()
	if r == nil {
	}
	var x !R = r
	_ = x
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

// Realism check against a stdlib interface.
func TestIface_StdlibReader(t *testing.T) {
	diags := checkTyped(t, `package main
import "io"
type T struct{}
func (t *T) Read(p []byte) (int, error) { return 0, nil }
func f(ord io.Reader) {
	var p *T = nil
	var ok !io.Reader = p
	var bad !io.Reader = ord
	_ = ok
	_ = bad
}`)
	mustGN001(t, diags, 1) // only the `ord` line
}
