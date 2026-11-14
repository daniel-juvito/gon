// Generic semantics (v1.8 / M3).
//
// Executable specification for docs/rfc-generic-semantics.md.
//
//	G2  — `!X` on a type parameter is valid iff X is a nilable type parameter
//	G3  — field contracts survive instantiation (Box[T]{})
//	G4  — `!` in a type-argument position is GN003
//	G5  — call contracts resolve through explicit and inferred instantiation
//	G6  — bodies checked once, opaquely
//	G7  — `var v !P` with no initializer is GN002 for a nilable P
//	G9  — `!` in a constraint is GN003
package checker

import (
	"go/ast"
	"go/parser"
	"testing"
)

func parseExprForTest(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

const genPreamble = `package main
type T struct{}
type A struct{}
type B struct{}
type Stringer interface{ String() string }
func mk() *T { return &T{} }
`

func genCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, genPreamble+body)
}

func want(t *testing.T, d []*Diagnostic, code string, n int) {
	t.Helper()
	if got := countCode(d, code); got != n {
		t.Fatalf("expected %d %s, got %d: %v", n, code, got, d)
	}
}

// --- G2 / §3: nilable type parameters --------------------------------------

func TestGen_NilableTypeParamsAccepted(t *testing.T) {
	cases := map[string]string{
		"tilde ptr":   `func F[P ~*T](p !P) {}`,
		"tilde slice": `func F[S ~[]T](s !S) {}`,
		"union ptrs":  `func F[P *A | *B](p !P) {}`,
		"tilde map":   `func F[M ~map[string]T](m !M) {}`,
		"tilde func":  `func F[C ~func()](f !C) {}`,
		"tilde chan":  `func F[C ~chan T](c !C) {}`,
		"ptr + iface": `func F[P *A | Stringer](p !P) {}`,
		"intersected": `func F[P interface{ ~int | ~*T; ~*T }](p !P) {}`,
		"with method": `func F[P interface{ ~*T; String() string }](p !P) {}`,
		"nested embed": `type Ptrs interface{ ~*T }
func F[P interface{ Ptrs }](p !P) {}`,
		"E from P": `func F[P ~*E, E any](p !P) {}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			d := genCheck(t, body)
			want(t, d, "GN003", 0)
		})
	}
}

func TestGen_NonNilableTypeParamsRejected(t *testing.T) {
	cases := map[string]string{
		"any":         `func F[X any](x !X) {}`,
		"comparable":  `func F[X comparable](x !X) {}`,
		"method only": `func F[X Stringer](x !X) {}`,
		"tilde int":   `func F[X ~int](x !X) {}`,
		"mixed":       `func F[X *A | int](x !X) {}`,
		"mixed tilde": `func F[X ~int | ~*T](x !X) {}`,
		"struct term": `func F[X T](x !X) {}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			d := genCheck(t, body)
			want(t, d, "GN003", 1)
		})
	}
}

func TestGen_BangOnTypeParamInResultAndField(t *testing.T) {
	d := genCheck(t, `func F[X any]() !X { var z X; return z }`)
	want(t, d, "GN003", 1)
	d = genCheck(t, `type S[X any] struct{ v !X }`)
	want(t, d, "GN003", 1)
	d = genCheck(t, `type S[X ~*T] struct{ v !X }`)
	want(t, d, "GN003", 0)
}

// --- G7 / G6: bodies -------------------------------------------------------

func TestGen_ZeroValueVarOfNilableTypeParam(t *testing.T) {
	d := genCheck(t, `func F[P ~*T]() { var v !P; _ = v }`)
	want(t, d, "GN002", 1)
	d = genCheck(t, `func F[P ~*T](p P) { var v !P = p; _ = v }`)
	want(t, d, "GN002", 0)
}

func TestGen_BodyNilIntoBangResult(t *testing.T) {
	d := genCheck(t, `func Nil[X any]() !*X { return nil }`)
	want(t, d, "GN001", 1)
}

func TestGen_BodyCheckedOnceRegardlessOfInstantiations(t *testing.T) {
	d := genCheck(t, `func Nil[X any]() !*X { return nil }
func main() { _ = Nil[int](); _ = Nil[string](); _ = Nil[T]() }`)
	want(t, d, "GN001", 1)
}

// --- G3: field contracts through instantiation -----------------------------

const boxDecl = `type Box[X any] struct{ v !*X }
`

func TestGen_InstantiatedZeroConstruction(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { _ = Box[T]{} }`)
	want(t, d, "GN002", 1)
}

func TestGen_InstantiatedNilField(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { _ = Box[T]{v: nil} }`)
	want(t, d, "GN001", 1)
}

func TestGen_InstantiatedOK(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { _ = Box[T]{v: mk()} }`)
	want(t, d, "GN001", 0)
	want(t, d, "GN002", 0)
}

func TestGen_InstantiatedVarDeclNoInit(t *testing.T) {
	d := genCheck(t, boxDecl+`var a Box[T]`)
	want(t, d, "GN002", 1)
}

func TestGen_MultiParamInstantiation(t *testing.T) {
	d := genCheck(t, `type Reg[K comparable, V any] struct{ items !map[K]V }
func f() { _ = Reg[string, T]{} }`)
	want(t, d, "GN002", 1)
}

func TestGen_NestedInstantiation(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { _ = Box[Box[T]]{} }`)
	want(t, d, "GN002", 1)
}

func TestGen_SelectorOnInstantiatedIsSource(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { b := Box[T]{v: mk()}; if b.v == nil {} }`)
	want(t, d, "GW001", 1)
}

func TestGen_MutationNilToInstantiatedField(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { b := Box[T]{v: mk()}; b.v = nil }`)
	want(t, d, "GN001", 1)
}

func TestGen_UnkeyedInstantiatedLiteral(t *testing.T) {
	d := genCheck(t, boxDecl+`func f() { _ = Box[T]{nil} }`)
	want(t, d, "GN001", 1)
}

func TestGen_ElementContractThroughGenericLiteral(t *testing.T) {
	d := genCheck(t, `type Lst[X any] struct{ items []!*X }
func f() { _ = Lst[T]{items: []!*T{nil}} }`)
	want(t, d, "GN001", 1)
}

func TestGen_NonGenericUnaffected(t *testing.T) {
	d := genCheck(t, `type P struct{ v !*T }
func f() { _ = P{} }`)
	want(t, d, "GN002", 1)
}

// --- G4 / G9: `!` in type arguments and constraints ------------------------

func TestGen_BangInTypeArgument(t *testing.T) {
	d := genCheck(t, boxDecl+`var d Box[!*T]`)
	want(t, d, "GN003", 1)
}

func TestGen_BangInExplicitCallTypeArgument(t *testing.T) {
	d := genCheck(t, `func F[X any]() {}
func g() { F[!*T]() }`)
	want(t, d, "GN003", 1)
}

func TestGen_BangInConstraint(t *testing.T) {
	d := genCheck(t, `func F[P ~!*T](p P) {}`)
	want(t, d, "GN003", 1)
}

func TestGen_BangInConstraintInterface(t *testing.T) {
	d := genCheck(t, `func F[S interface{ ~[]!*T }](s S) {}`)
	// element ! inside a constraint element: reserved.
	want(t, d, "GN003", 1)
}

// --- G5: calls through instantiation ---------------------------------------

func TestGen_ExplicitCallResultIsSource(t *testing.T) {
	d := genCheck(t, `func NN[X any]() !*X { return new(X) }
func f() { p := NN[T](); if p == nil {} }`)
	want(t, d, "GW001", 1)
}

func TestGen_InferredCallResultIsSource(t *testing.T) {
	d := genCheck(t, `func Must[V any](p *V) !*V { return p }
func f() { m := Must(mk()); if m == nil {} }`)
	want(t, d, "GW001", 1)
}

func TestGen_NilArgToBangParamExplicit(t *testing.T) {
	d := genCheck(t, `func Use[X any](p !*X) {}
func f() { Use[T](nil) }`)
	want(t, d, "GN001", 1)
}

func TestGen_NilArgToBangParamInferred(t *testing.T) {
	d := genCheck(t, `func Use[X any](p !*X, q *X) {}
func f() { Use(nil, mk()) }`)
	want(t, d, "GN001", 1)
}

func TestGen_IdentityDoesNotCarryContract(t *testing.T) {
	d := genCheck(t, `func Id[X any](x X) X { return x }
func NN() !*T { return mk() }
func f() { q := Id(NN()); if q == nil {} }`)
	want(t, d, "GW001", 0)
}

// Source-declared method result contracts are not tracked in Gon v1.x for any
// receiver (only .gna carries method contracts; generic .gna is M5b / v1.9).
// A generic receiver must behave exactly like a non-generic one.
func TestGen_MethodResultsMatchNonGeneric(t *testing.T) {
	g := genCheck(t, boxDecl+`func (b Box[X]) Get() !*X { return b.v }
func f() { b := Box[T]{v: mk()}; g := b.Get(); if g == nil {} }`)
	n := genCheck(t, `type NBox struct{ v !*T }
func (b NBox) Get() !*T { return b.v }
func f() { b := NBox{v: mk()}; g := b.Get(); if g == nil {} }`)
	if countCode(g, "GW001") != countCode(n, "GW001") {
		t.Fatalf("generic receiver diverges from non-generic: generic=%v non-generic=%v", g, n)
	}
}

func TestGen_BangOnTypeParamElement(t *testing.T) {
	d := genCheck(t, `func F[X any]() { _ = []!X{} }`)
	want(t, d, "GN003", 1)
	d = genCheck(t, `func F[X ~*T]() { _ = []!X{} }`)
	want(t, d, "GN003", 0)
}

// --- regression: ordinary generics are untouched ---------------------------

func TestGen_OrdinaryGenericsClean(t *testing.T) {
	d := genCheck(t, `func Id[X any](x X) X { return x }
type Pair[A, B any] struct{ a A; b B }
func f() { _ = Pair[int, string]{}; _ = Id(1) }`)
	if len(d) != 0 {
		t.Fatalf("ordinary generics must be clean: %v", d)
	}
}

// --- G10: external generics stay as v1.7.1 ---------------------------------

// `.gna` application to an instantiation of an external generic is deferred
// to M5b; an explicit instantiation of an imported symbol must not start
// resolving contracts in v1.8.
func TestGen_ExternalInstantiationNotResolved(t *testing.T) {
	d := checkTyped(t, `package main
import "slices"
func f() { _ = slices.Clone[[]int](nil) }`)
	for _, x := range d {
		if x.Code == "GN001" || x.Code == "GN002" {
			t.Fatalf("external instantiation must not resolve contracts: %v", d)
		}
	}
}

func TestGen_GenericOriginUnit(t *testing.T) {
	// pkg.Box[T] is left alone (G10); Box[T] / Reg[K, V] resolve to the ident.
	if got := genericOrigin(parseExprForTest(t, "Box[T]")); typeExprName(got) != "Box" {
		t.Fatalf("Box[T] -> %q", typeExprName(got))
	}
	if got := genericOrigin(parseExprForTest(t, "Reg[K, V]")); typeExprName(got) != "Reg" {
		t.Fatalf("Reg[K,V] -> %q", typeExprName(got))
	}
	e := parseExprForTest(t, "pkg.Box[T]")
	if genericOrigin(e) != e {
		t.Fatalf("pkg.Box[T] must be returned unchanged")
	}
}

// --- G8: generic interfaces behave like non-generic ones -------------------

// v1.4 D5 / I3: `!` never alters method sets or interface implementation, so
// `!` on an interface method signature is not a satisfaction constraint.
// A generic interface must behave exactly like its non-generic counterpart.
func TestGen_GenericInterfaceMatchesNonGeneric(t *testing.T) {
	g := genCheck(t, `type Getter[X any] interface{ Get() !*X }
type Impl struct{}
func (Impl) Get() *T { return &T{} }
func use(g Getter[T]) {}
func f() { use(Impl{}) }`)
	n := genCheck(t, `type Getter interface{ Get() !*T }
type Impl struct{}
func (Impl) Get() *T { return &T{} }
func use(g Getter) {}
func f() { use(Impl{}) }`)
	if len(g) != len(n) {
		t.Fatalf("generic interface diverges from non-generic: generic=%v non-generic=%v", g, n)
	}
	want(t, g, "GN001", 0)
}

func TestGen_BangGenericInterfaceValue(t *testing.T) {
	// `!I` over an instantiated interface keeps the v1.4 rule: concrete is
	// accepted, nil is GN001.
	d := genCheck(t, `type Getter[X any] interface{ Get() *X }
type Impl struct{}
func (Impl) Get() *T { return &T{} }
func f() { var a !Getter[T] = Impl{}; _ = a }`)
	want(t, d, "GN001", 0)
	d = genCheck(t, `type Getter[X any] interface{ Get() *X }
func f() { var a !Getter[T] = nil; _ = a }`)
	want(t, d, "GN001", 1)
}
