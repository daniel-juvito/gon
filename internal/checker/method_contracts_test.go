// Call-site contracts for source-declared methods (v1.11 / M7).
//
// Executable specification for docs/rfc-method-contracts.md.
//
//	S1 — a call is matched to a declaration by go/types object identity, never
//	     by name
//	S2 — direct calls: `!` parameters (GN001) and `!` results (a non-nil source)
//	S3 — method expressions, with the receiver offset
//	S4 — promotion through embedding resolves to the declaring method
//	S5 — a call through an interface uses the interface's declaration
//	S6 — a method value carries no contract
//	S7 — generic receivers resolve through Origin()
//	S9 — only declarations in the checked files are consulted (a file, or the
//	     package under M8)
//	S10 — without type information nothing is applied
package checker

import (
	"strings"
	"testing"
)

const mcPre = `package main

type T struct{}
type Box struct{}
type Other struct{}

`

func mcCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, mcPre+body)
}

func mcWant(t *testing.T, diags []*Diagnostic, gn001, gw001 int) {
	t.Helper()
	if g := countCode(diags, "GN001"); g != gn001 {
		t.Fatalf("GN001 = %d, want %d: %v", g, gn001, diags)
	}
	if g := countCode(diags, "GW001"); g != gw001 {
		t.Fatalf("GW001 = %d, want %d: %v", g, gw001, diags)
	}
	if n := len(diags); n != gn001+gw001 {
		t.Fatalf("unexpected extra diagnostics: %v", diags)
	}
}

// --- S2: direct calls ------------------------------------------------------

func TestMethodContractNilToPointerParameter(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p !*T) {}
func f(b *Box) { b.Set(nil) }`)
	mcWant(t, d, 1, 0)
	if !strings.Contains(d[0].Message, "Box.Set") {
		t.Fatalf("the message should name the method: %v", d[0])
	}
}

func TestMethodContractNilToEveryNilableKind(t *testing.T) {
	d := mcCheck(t, `
type I interface{ M() }
func (b *Box) Ptr(p !*T)            {}
func (b *Box) Slice(p ![]int)       {}
func (b *Box) Map(p !map[string]int) {}
func (b *Box) Iface(p !I)           {}
func (b *Box) Func(p !func())       {}
func f(b *Box) {
	b.Ptr(nil)
	b.Slice(nil)
	b.Map(nil)
	b.Iface(nil)
	b.Func(nil)
}`)
	mcWant(t, d, 5, 0)
}

func TestMethodContractOrdinaryInterfaceValueToBangInterfaceParameter(t *testing.T) {
	d := mcCheck(t, `
type I interface{ M() }
func (b *Box) Take(p !I) {}
func f(b *Box, v I) { b.Take(v) }`)
	mcWant(t, d, 1, 0)
	if !strings.Contains(d[0].Message, "ordinary interface value") {
		t.Fatalf("D3b wording expected: %v", d[0])
	}
}

func TestMethodContractOnlyTheAnnotatedPositionIsChecked(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Two(a *T, p !*T) {}
func f(b *Box) {
	b.Two(nil, &T{})
	b.Two(&T{}, nil)
}`)
	mcWant(t, d, 1, 0)
	if !strings.Contains(d[0].Message, "argument 2") {
		t.Fatalf("expected position 2: %v", d[0])
	}
}

func TestMethodContractValueReceiver(t *testing.T) {
	d := mcCheck(t, `
func (b Box) ValSet(p !*T) {}
func f(b Box, pb *Box) { b.ValSet(nil); pb.ValSet(nil) }`)
	mcWant(t, d, 2, 0)
}

func TestMethodContractNonNilArgumentIsAccepted(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p !*T) {}
func f(b *Box, t *T) { b.Set((!*T)(t)); b.Set(&T{}) }`)
	mcWant(t, d, 0, 0)
}

func TestMethodContractResultIsANonNilSource(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Get() !*T { return &T{} }
func f(b *Box) {
	g := b.Get()
	if g == nil {}
	var h = b.Get()
	if h != nil {}
}`)
	mcWant(t, d, 0, 2)
}

func TestMethodContractMultiValueResult(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Load() (!*T, error) { return &T{}, nil }
func f(b *Box) {
	a, err := b.Load()
	if a == nil {}
	_ = err
}`)
	mcWant(t, d, 0, 1)
}

// An ordinary (unannotated) result stays ordinary.
func TestMethodContractUnannotatedResultIsNotASource(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Get() *T { return nil }
func f(b *Box) { g := b.Get(); if g == nil {} }`)
	mcWant(t, d, 0, 0)
}

// --- S1: object identity, never name --------------------------------------

func TestMethodContractTwoTypesSharingAMethodName(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Put(p !*T) {}
func (o *Other) Put(p *T) {}
func f(b *Box, o *Other) {
	b.Put(nil)
	o.Put(nil)
}`)
	mcWant(t, d, 1, 0)
}

func TestMethodContractTwoTypesSharingAMethodNameDeclarationOrderSwapped(t *testing.T) {
	d := mcCheck(t, `
func (o *Other) Put(p *T) {}
func (b *Box) Put(p !*T) {}
func f(b *Box, o *Other) {
	o.Put(nil)
	b.Put(nil)
}`)
	mcWant(t, d, 1, 0)
}

func TestMethodContractMethodAndPlainFunctionShareAName(t *testing.T) {
	d := mcCheck(t, `
func Put(p *T) {}
func (b *Box) Put(p !*T) {}
func f(b *Box) {
	Put(nil)
	b.Put(nil)
}`)
	mcWant(t, d, 1, 0)

	d = mcCheck(t, `
func (b *Box) Put(p !*T) {}
func Put(p *T) {}
func f(b *Box) {
	b.Put(nil)
	Put(nil)
}`)
	mcWant(t, d, 1, 0)
}

func TestMethodContractMethodWithoutBangSharingAFunctionNameWithBang(t *testing.T) {
	d := mcCheck(t, `
func Put(p !*T) {}
func (b *Box) Put(p *T) {}
func f(b *Box) {
	Put(nil)
	b.Put(nil)
}`)
	mcWant(t, d, 1, 0)
}

// --- S3: method expressions -----------------------------------------------

func TestMethodContractMethodExpression(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p !*T) {}
func (b Box) ValSet(p !*T) {}
func f(b *Box, v Box) {
	(*Box).Set(b, nil)
	Box.ValSet(v, nil)
	(*Box).ValSet(b, nil)
}`)
	mcWant(t, d, 3, 0)
	for _, x := range d {
		if !strings.Contains(x.Message, "argument 1") {
			t.Fatalf("the receiver offset must not shift the reported position: %v", x)
		}
	}
}

func TestMethodContractMethodExpressionReceiverArgumentIsNotAContract(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p !*T) {}
func f() { (*Box).Set(nil, &T{}) }`)
	mcWant(t, d, 0, 0)
}

// --- S4: promotion through embedding --------------------------------------

func TestMethodContractPromotedThroughOneAndTwoLevels(t *testing.T) {
	d := mcCheck(t, `
type W1 struct{ Box }
type W2 struct{ W1 }
type W3 struct{ *Box }
func (b *Box) Set(p !*T) {}
func (b *Box) Get() !*T { return &T{} }
func f(w W1, w2 W2, w3 W3) {
	w.Set(nil)
	w2.Set(nil)
	w3.Set(nil)
	g := w2.Get()
	if g == nil {}
}`)
	mcWant(t, d, 3, 1)
}

// An outer method hides the promoted one: the outer declaration applies.
func TestMethodContractOuterMethodShadowsPromoted(t *testing.T) {
	d := mcCheck(t, `
type W struct{ Box }
func (b *Box) Set(p !*T) {}
func (w *W) Set(p *T) {}
func f(w *W) { w.Set(nil) }`)
	mcWant(t, d, 0, 0)
}

// --- S5: calls through an interface ---------------------------------------

func TestMethodContractThroughInterfaceUsesTheInterfaceDeclaration(t *testing.T) {
	d := mcCheck(t, `
type Setter interface{ Set(p !*T) }
func (b *Box) Set(p *T) {} // implementation need not repeat the !
var _ Setter = (*Box)(nil)
func f(s Setter, b *Box) {
	s.Set(nil) // through the interface: checked
	b.Set(nil) // concrete declaration has no !: accepted
}`)
	mcWant(t, d, 1, 0)
	if !strings.Contains(d[0].Message, "Setter.Set") {
		t.Fatalf("the message should name the interface method: %v", d[0])
	}
}

func TestMethodContractThroughConcreteTypeUsesTheConcreteDeclaration(t *testing.T) {
	d := mcCheck(t, `
type Setter interface{ Set(p *T) }
func (b *Box) Set(p !*T) {}
func f(s Setter, b *Box) {
	s.Set(nil) // the interface has no !: accepted
	b.Set(nil) // the concrete declaration has one
}`)
	mcWant(t, d, 1, 0)
}

func TestMethodContractInterfaceResultIsASource(t *testing.T) {
	d := mcCheck(t, `
type Getter interface{ Get() !*T }
func f(g Getter) { v := g.Get(); if v == nil {} }`)
	mcWant(t, d, 0, 1)
}

func TestMethodContractEmbeddedInterface(t *testing.T) {
	d := mcCheck(t, `
type Setter interface{ Set(p !*T) }
type Both interface {
	Setter
	Close()
}
func f(b Both) { b.Set(nil) }`)
	mcWant(t, d, 1, 0)
}

func TestMethodContractAnonymousInterface(t *testing.T) {
	d := mcCheck(t, `
func f(s interface{ Set(p !*T) }) { s.Set(nil) }`)
	mcWant(t, d, 1, 0)
}

// --- S6: method values carry nothing --------------------------------------

func TestMethodContractMethodValueIsNotTracked(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p !*T) {}
func apply(fn func(*T)) { fn(nil) }
func f(b *Box) {
	set := b.Set
	set(nil)
	apply(b.Set)
	var g func(*T) = b.Set
	g(nil)
}`)
	mcWant(t, d, 0, 0)
}

// --- S7: generic receivers ------------------------------------------------

func TestMethodContractGenericReceiver(t *testing.T) {
	d := mcCheck(t, `
type G[V any] struct{}
func (g *G[V]) Put(p !*T) {}
func (g *G[V]) Get() !*T { return &T{} }
func f(g *G[int]) {
	g.Put(nil)
	v := g.Get()
	if v == nil {}
}`)
	mcWant(t, d, 1, 1)
}

func TestMethodContractGenericInterface(t *testing.T) {
	d := mcCheck(t, `
type S[V any] interface{ Put(p !*V) }
func f(s S[int]) { s.Put(nil) }`)
	mcWant(t, d, 1, 0)
}

// --- bodies are unaffected ------------------------------------------------

func TestMethodContractBodyChecksStillHold(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Get() !*T { return nil }
func (b *Box) Set(p !*T) { if p == nil {} }`)
	mcWant(t, d, 1, 1)
}

// --- S10: degradation ------------------------------------------------------

func TestMethodContractWithoutTypeInformationNothingIsApplied(t *testing.T) {
	// checkSource builds a checker without go/types: no method is resolved, and
	// a name is never used as a guess.
	d := checkSource(t, mcPre+`
func (b *Box) Set(p !*T) {}
func f(b *Box) { b.Set(nil) }`)
	if len(d) != 0 {
		t.Fatalf("no type information must mean no method contract: %v", d)
	}
}

// A method without any `!` produces no diagnostic, as before v1.11.
func TestMethodContractNoBangAnywhereIsUnchanged(t *testing.T) {
	d := mcCheck(t, `
func (b *Box) Set(p *T) {}
func (b *Box) Get() *T { return nil }
func f(b *Box) { b.Set(nil); g := b.Get(); if g == nil {} }`)
	mcWant(t, d, 0, 0)
}

// --- S9: the files checked together ---------------------------------------

// In package mode a method declared in a sibling file is checked at its callers
// (M8 shares the files; M7 adds no cross-file mechanism of its own).
func TestMethodContractAcrossFilesInPackageMode(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": "package pk\n\ntype T struct{}\ntype Box struct{}\ntype W struct{ Box }\n\nfunc (b *Box) Set(p !*T) {}\nfunc (b *Box) Get() !*T { return &T{} }\n",
		"b.gon": "package pk\n\nfunc use(b *Box, w W) {\n\tb.Set(nil)\n\tw.Set(nil)\n\tg := b.Get()\n\tif g == nil {\n\t}\n}\n",
	})
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(pkgCodes(res, "b.gon"), ",")
	if got != "GN001,GN001,GW001" {
		t.Fatalf("b.gon: %s (a.gon: %v)", got, pkgCodes(res, "a.gon"))
	}
	if a := pkgCodes(res, "a.gon"); len(a) != 0 {
		t.Fatalf("a.gon should be clean: %v", a)
	}
}

// `!` offsets belong to one file: a method declared in a sibling file is read
// against that file's offsets, not the caller's.
func TestMethodContractSiblingDeclarationUsesItsOwnOffsets(t *testing.T) {
	// a.gon's `!` sits at one byte offset; b.gon has the same text at the same
	// offset WITHOUT a `!`. A lookup against the wrong file's offsets would
	// either miss a.gon's contract or invent one for b.gon's method.
	dir := pkgDir(t, map[string]string{
		"a.gon": "package pk\n\ntype T struct{}\ntype A struct{}\n\nfunc (x *A) Set(p !*T) {}\n",
		"b.gon": "package pk\n\ntype B struct{}\n\nfunc (x *B) Set(p  *T) {}\n\nfunc use(a *A, b *B) {\n\ta.Set(nil)\n\tb.Set(nil)\n}\n",
	})
	res, err := checkPkg(t, dir, "a.gon", "b.gon")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(pkgCodes(res, "b.gon"), ","); got != "GN001" {
		t.Fatalf("exactly a.Set(nil) must be reported, got %q", got)
	}
}

// A method declared in a plain .go file of the package has no `!`: not a contract.
func TestMethodContractPlainGoSiblingHasNoContract(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon":  "package pk\n\nfunc use(b *Box) { b.Set(nil) }\n",
		"box.go": "package pk\n\ntype T struct{}\ntype Box struct{}\n\nfunc (b *Box) Set(p *T) {}\n",
	})
	res, err := checkPkg(t, dir, "a.gon")
	if err != nil {
		t.Fatal(err)
	}
	if a := pkgCodes(res, "a.gon"); len(a) != 0 {
		t.Fatalf("a method of a plain .go file is an ordinary method: %v", a)
	}
}

// In FILE mode a method declared in another file is simply not visible: there is
// no cross-file mechanism, and the call is not guessed by name.
func TestMethodContractFileModeDoesNotSeeSiblingFiles(t *testing.T) {
	d := checkTyped(t, "package main\n\nfunc use(b *Box) { b.Set(nil) }\n")
	if countCode(d, "GN001") != 0 {
		t.Fatalf("a method of another file must not be consulted in file mode: %v", d)
	}
}

// S7: a method that declares its own type parameters (Go 1.27).
func TestMethodContractMethodWithOwnTypeParameters(t *testing.T) {
	d := mcCheck(t, `
type G[V any] struct{}
func (g G[V]) Map[Y any](p !*Y) !*Y { return p }
func f(g G[int], x *int) {
	g.Map(nil)
	r := g.Map((!*int)(x))
	if r == nil {}
}`)
	mcWant(t, d, 1, 1)
}
