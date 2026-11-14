// Generic `.gna` and external generic contract application (v1.9 / M5b).
//
// Executable specification for docs/rfc-generic-gna.md.
//
//	X2  — explicit instantiation of an external generic resolves like the
//	      inferred form
//	X3  — every `!` flag is validated against the ORIGIN signature; an invalid
//	      flag is GN003 and only that flag is dropped (Q3)
//	X4  — generic receivers / `types:` resolve through instantiation
//	X7  — X3 applies to type parameters a method declares itself (Go 1.27)
//	X10 — every bundled `.gna` passes validation (release gate, Q5)
package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
)

const ggPre = `package main
import "github.com/daniel-juvito/gon/internal/extlib"
`

// gg checks body against the BUNDLED annotations.
func gg(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return ecoCheck(t, ggPre+body)
}

// ggWith checks body against a custom extlib annotation body.
func ggWith(t *testing.T, yaml, body string) []*Diagnostic {
	t.Helper()
	return ecoCheckWith(t, ggPre+body, badRegistry(t, yaml))
}

func ggWant(t *testing.T, d []*Diagnostic, code string, n int) {
	t.Helper()
	if got := countCode(d, code); got != n {
		t.Fatalf("expected %d %s, got %d: %v", n, code, got, d)
	}
}

func ggMsg(t *testing.T, d []*Diagnostic, code, substr string) {
	t.Helper()
	for _, x := range d {
		if x.Code == code && strings.Contains(x.Message, substr) {
			return
		}
	}
	t.Fatalf("expected a %s containing %q, got %v", code, substr, d)
}

// --- the bundled generic annotations are clean ------------------------------

func TestGGna_BundledGenericAnnotationsValidate(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Pick[[]int]; _ = extlib.Unwrap[int] }`)
	ggWant(t, d, "GN003", 0)
	ggWant(t, d, "GW004", 0)
}

// --- X2: explicit instantiation resolves like inference ---------------------

func TestGGna_ExplicitResultIsSource(t *testing.T) {
	d := gg(t, `func f(p *int) { r := extlib.Unwrap[int](p); if r == nil {} }`)
	ggWant(t, d, "GW001", 1)
}

func TestGGna_InferredResultIsSource(t *testing.T) {
	d := gg(t, `func f(p *int) { r := extlib.Unwrap(p); if r == nil {} }`)
	ggWant(t, d, "GW001", 1)
}

func TestGGna_ExplicitEqualsInferred(t *testing.T) {
	exp := gg(t, `func f(p *int) { r := extlib.Unwrap[int](p); if r == nil {} }`)
	inf := gg(t, `func f(p *int) { r := extlib.Unwrap(p); if r == nil {} }`)
	if len(exp) != len(inf) {
		t.Fatalf("explicit and inferred diverge: explicit=%v inferred=%v", exp, inf)
	}
}

func TestGGna_ExplicitNilArgToBangParam(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Unwrap[int](nil) }`)
	ggWant(t, d, "GN001", 1)
}

func TestGGna_ExplicitNilArgPartialInstantiation(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Pick[[]int](nil) }`)
	ggWant(t, d, "GN001", 1)
}

func TestGGna_ExternalGenericStructZero(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Box[int]{} }`)
	ggWant(t, d, "GN002", 1)
}

func TestGGna_ExternalGenericStructNilField(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Box[int]{V: nil} }`)
	ggWant(t, d, "GN001", 1)
}

func TestGGna_ExternalGenericStructOK(t *testing.T) {
	d := gg(t, `func f(p *int) { _ = extlib.Box[int]{V: p} }`)
	ggWant(t, d, "GN001", 0)
	ggWant(t, d, "GN002", 0)
}

func TestGGna_NewExternalGenericStruct(t *testing.T) {
	d := gg(t, `func f() { _ = new(extlib.Box[int]) }`)
	ggWant(t, d, "GN002", 1)
}

func TestGGna_NestedExternalInstantiation(t *testing.T) {
	d := gg(t, `func f() { _ = extlib.Box[extlib.Box[int]]{} }`)
	ggWant(t, d, "GN002", 1)
}

// --- X4: generic receivers and field use ------------------------------------

func TestGGna_MethodResultThroughInstantiatedReceiver(t *testing.T) {
	d := gg(t, `func f(p *int) {
	b := extlib.Box[int]{V: p}
	g := b.Get()
	if g == nil {}
}`)
	ggWant(t, d, "GW001", 1)
}

func TestGGna_FieldSelectorOnInstantiatedIsSource(t *testing.T) {
	d := gg(t, `func f(p *int) {
	b := extlib.Box[int]{V: p}
	if b.V == nil {}
}`)
	ggWant(t, d, "GW001", 1)
}

// --- X3: validation against the origin signature ----------------------------

func TestGGna_BangOnNonNilableTypeParam(t *testing.T) {
	d := ggWith(t, `functions:
  Ident:
    params:
      - "!X"
    results:
      - "X"
`, `func f() { _ = extlib.Ident(1) }`)
	ggWant(t, d, "GN003", 1)
	ggMsg(t, d, "GN003", "type parameter X")
}

func TestGGna_BangOnNilableTypeParamAccepted(t *testing.T) {
	d := ggWith(t, `functions:
  Pick:
    params:
      - "!S"
    results:
      - "!S"
`, `func f() { _ = extlib.Pick([]int{1}) }`)
	ggWant(t, d, "GN003", 0)
}

func TestGGna_BangOnNonNilableConcreteKind(t *testing.T) {
	d := ggWith(t, `functions:
  Echo:
    params:
      - "!string"
    results:
      - "string"
`, `func f() { _ = extlib.Echo("") }`)
	ggWant(t, d, "GN003", 1)
	ggMsg(t, d, "GN003", "parameter 1 (s)")
	ggMsg(t, d, "GN003", "not a nilable kind")
}

func TestGGna_BangOnNonNilableResult(t *testing.T) {
	d := ggWith(t, `functions:
  Echo:
    params:
      - "string"
    results:
      - "!string"
`, `func f() { _ = extlib.Echo("") }`)
	ggWant(t, d, "GN003", 1)
	ggMsg(t, d, "GN003", "result 1")
}

// Q3: only the offending flag is dropped; the entry's other positions keep
// their contract.
func TestGGna_PerPositionDrop(t *testing.T) {
	yaml := `functions:
  Mix:
    params:
      - "!string"
      - "!*int"
    results:
      - "error"
`
	d := ggWith(t, yaml, `func f() { _ = extlib.Mix("", nil) }`)
	ggWant(t, d, "GN003", 1) // the string flag
	ggWant(t, d, "GN001", 1) // the *int flag still applies
	ggMsg(t, d, "GN001", "argument 2")

	// A nil in the dropped position is no longer a contract violation.
	d = ggWith(t, yaml, `func f(p *int) { _ = extlib.Mix(nil, p) }`)
	ggWant(t, d, "GN001", 0)
}

// Arity mismatch still drops the WHOLE entry (M5a, unchanged).
func TestGGna_ArityMismatchStillDropsWholeEntry(t *testing.T) {
	d := ggWith(t, `functions:
  Mix:
    params:
      - "!*int"
    results:
      - "error"
`, `func f() { _ = extlib.Mix("", nil) }`)
	ggWant(t, d, "GN003", 1)
	ggWant(t, d, "GN001", 0)
}

func TestGGna_FieldFlagValidation(t *testing.T) {
	yaml := `types:
  Meta:
    fields:
      Label: "!string"
      H: "!*Handle"
`
	d := ggWith(t, yaml, `func f() { _ = extlib.Meta{} }`)
	ggWant(t, d, "GN003", 1) // Label
	ggMsg(t, d, "GN003", "field Meta.Label")
	ggWant(t, d, "GN002", 1) // only H is still required
	ggMsg(t, d, "GN002", "Meta.H")
}

// The registry's own file object is never mutated: a second check on the same
// registry reports the same diagnostics (a long-lived LSP registry).
func TestGGna_RegistryNotMutatedAcrossRuns(t *testing.T) {
	reg := badRegistry(t, `functions:
  Echo:
    params:
      - "!string"
    results:
      - "string"
`)
	src := ggPre + `func f() { _ = extlib.Echo("") }`
	first := ecoCheckWith(t, src, reg)
	second := ecoCheckWith(t, src, reg)
	ggWant(t, first, "GN003", 1)
	ggWant(t, second, "GN003", 1)
}

// --- X7: method-level type parameters (Go 1.27) -----------------------------

func TestGGna_MethodLevelTypeParamNotNilable(t *testing.T) {
	d := ggWith(t, `methods:
  Box.Echo:
    params:
      - "!Y"
    results:
      - "Y"
`, `func f() { var b extlib.Box[int]; _ = b }`)
	ggWant(t, d, "GN003", 1)
	ggMsg(t, d, "GN003", "type parameter Y")
}

func TestGGna_MethodLevelTypeParamNilable(t *testing.T) {
	// Box.Ptr declares P ~*X: bundled annotation already flags it.
	d := gg(t, `func f() { var b extlib.Box[int]; _ = b }`)
	ggWant(t, d, "GN003", 0)
}

// --- X10: every bundled `.gna` passes validation (release gate) -------------

func TestGGna_BundledAnnotationsAudit(t *testing.T) {
	root := filepath.Join("..", "..", "annotations")
	var files []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".gna") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) < 3 {
		t.Fatalf("expected the bundled annotations, found %v", files)
	}
	for _, f := range files {
		gf, err := gna.Load(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		src := "package main\nimport _ \"" + gf.Package + "\"\n"
		d := ecoCheck(t, src)
		for _, x := range d {
			if x.Code == "GN003" || x.Code == "GW004" {
				t.Errorf("%s (%s): bundled annotation fails validation: %v", f, gf.Package, x)
			}
		}
	}
}

// Regression: annotated INTERFACE methods (`io.Reader.Read`, …) are found. A
// pointer to an interface has an empty method set, so every such entry was a
// false GW004 and a plain `import "io"` produced three spurious warnings.
func TestGGna_InterfaceMethodsAreFound(t *testing.T) {
	d := ecoCheck(t, "package main\nimport \"io\"\nvar _ io.Reader\n")
	ggWant(t, d, "GW004", 0)
	ggWant(t, d, "GN003", 0)
}
