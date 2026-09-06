// Ecosystem contract expansion (v1.5 / M4a + M4b + M5a).
//
// Executable specification for docs/rfc-ecosystem-contract-expansion.md.
// Fixtures use the in-module reference package
// github.com/daniel-juvito/gon/internal/extlib and its checked-in
// annotations/.../extlib.gna.
package checker

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

func ecoCheck(t *testing.T, src string) []*Diagnostic {
	t.Helper()
	resolver := &gna.DirResolver{Root: filepath.Join("..", "..", "annotations")}
	return ecoCheckWith(t, src, resolver)
}

func ecoCheckWith(t *testing.T, src string, resolver gna.Resolver) []*Diagnostic {
	t.Helper()
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return c.Check()
}

func mustCode(t *testing.T, diags []*Diagnostic, code string) {
	t.Helper()
	if countCode(diags, code) == 0 {
		t.Fatalf("expected at least one %s, got %v", code, diags)
	}
}

func mustNoCode(t *testing.T, diags []*Diagnostic, code string) {
	t.Helper()
	if countCode(diags, code) != 0 {
		t.Fatalf("expected no %s, got %v", code, diags)
	}
}

// ---- M4a: external field contracts ----------------------------------------

func TestEcoExternalKeyedMissingField(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f(h *extlib.Handle) {
	_ = extlib.Conn{DB: h}
}
`
	mustCode(t, ecoCheck(t, src), "GN002") // Log and W missing
}

func TestEcoExternalKeyedExplicitNil(t *testing.T) {
	src := `package main
import (
	"bytes"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(h *extlib.Handle) {
	_ = extlib.Conn{DB: h, Log: nil, W: &bytes.Buffer{}}
}
`
	diags := ecoCheck(t, src)
	mustCode(t, diags, "GN001") // Log: nil
	mustNoCode(t, diags, "GN002")
}

func TestEcoExternalKeyedAllProvidedClean(t *testing.T) {
	src := `package main
import (
	"bytes"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(h *extlib.Handle) {
	_ = extlib.Conn{DB: h, Log: h, W: &bytes.Buffer{}}
}
`
	diags := ecoCheck(t, src)
	mustNoCode(t, diags, "GN001")
	mustNoCode(t, diags, "GN002")
}

func TestEcoExternalNewZeroValue(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	_ = new(extlib.Conn)
}
`
	mustCode(t, ecoCheck(t, src), "GN002")
}

func TestEcoExternalEmptyLiteralZeroValue(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	_ = &extlib.Conn{}
	_ = extlib.Conn{}
}
`
	if got := countCode(ecoCheck(t, src), "GN002"); got < 2 {
		t.Fatalf("expected GN002 for both zero-value sites, got %d", got)
	}
}

func TestEcoExternalUnkeyedFirewall(t *testing.T) {
	src := `package main
import (
	"bytes"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(h *extlib.Handle) {
	_ = extlib.Conn{h, h, &bytes.Buffer{}}
}
`
	diags := ecoCheck(t, src)
	mustNoCode(t, diags, "GN001")
	mustNoCode(t, diags, "GN002")
}

func TestEcoExternalFieldAsNonNilSource(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f(c *extlib.Conn) {
	if c.DB == nil {
	}
	var h !*extlib.Handle = c.DB
	_ = h
}
`
	diags := ecoCheck(t, src)
	mustCode(t, diags, "GW001")
	mustNoCode(t, diags, "GN001")
}

func TestEcoExternalFieldMutationNil(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f(c *extlib.Conn) {
	c.DB = nil
}
`
	mustCode(t, ecoCheck(t, src), "GN001")
}

func TestEcoExternalEmbeddedPromotion(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f(h *extlib.Handle) {
	_ = extlib.Wrapped{Name: h}
}
`
	diags := ecoCheck(t, src)
	// Base.Root is promoted and left unset.
	mustCode(t, diags, "GN002")
	found := false
	for _, d := range diags {
		if d.Code == "GN002" && strings.Contains(d.Message, "Base.Root") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected GN002 naming Base.Root, got %v", diags)
	}
}

// ---- M4b: interface-typed external positions ------------------------------

func TestEcoExternalInterfaceResultSource(t *testing.T) {
	src := `package main
import (
	"io"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f() {
	r := extlib.Open()
	if r == nil {
	}
	var x !io.Reader = extlib.Open()
	_ = x
	_ = r
}
`
	diags := ecoCheck(t, src)
	mustCode(t, diags, "GW001")
	mustNoCode(t, diags, "GN001")
}

func TestEcoExternalInterfaceFieldOrdinaryRejected(t *testing.T) {
	src := `package main
import (
	"io"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(w io.Writer, h *extlib.Handle) {
	_ = extlib.Conn{DB: h, Log: h, W: w}
}
`
	mustCode(t, ecoCheck(t, src), "GN001") // ordinary io.Writer into !io.Writer field
}

func TestEcoExternalInterfaceFieldTypedNilClean(t *testing.T) {
	src := `package main
import (
	"bytes"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(h *extlib.Handle) {
	_ = extlib.Conn{DB: h, Log: h, W: (*bytes.Buffer)(nil)}
}
`
	mustNoCode(t, ecoCheck(t, src), "GN001") // interface value non-nil; dynamic value not checked
}

func TestEcoExternalInterfaceFieldMutationOrdinary(t *testing.T) {
	src := `package main
import (
	"io"
	"github.com/daniel-juvito/gon/internal/extlib"
)
func f(c *extlib.Conn, w io.Writer) {
	c.W = w
}
`
	mustCode(t, ecoCheck(t, src), "GN001")
}

// ---- M5a: .gna validation against the real package -----------------------

func badRegistry(t *testing.T, body string) *gna.Registry {
	t.Helper()
	f, err := gna.LoadBytes("extlib.gna", []byte(
		"schema: 1\npackage: github.com/daniel-juvito/gon/internal/extlib\n"+body))
	if err != nil {
		t.Fatal(err)
	}
	reg := gna.NewRegistry()
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestEcoGnaArityMismatchParams(t *testing.T) {
	reg := badRegistry(t, `functions:
  Take:
    params:
      - "!string"
      - "!string"
    results:
      - "error"
`)
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	diags := ecoCheckWith(t, src, reg)
	mustCode(t, diags, "GN003")
	mustNoCode(t, diags, "GN001") // dropped contract → no downstream nil rejection
}

func TestEcoGnaArityMismatchResults(t *testing.T) {
	reg := badRegistry(t, `functions:
  Take:
    params:
      - "!string"
    results:
      - "error"
      - "error"
`)
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	diags := ecoCheckWith(t, src, reg)
	mustCode(t, diags, "GN003")
	mustNoCode(t, diags, "GN001")
}

func TestEcoGnaUnknownSymbolGW004(t *testing.T) {
	reg := badRegistry(t, `functions:
  Take:
    params:
      - "!string"
    results:
      - "error"
  Nonexistent:
    params:
      - "int"
`)
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	diags := ecoCheckWith(t, src, reg)
	mustCode(t, diags, "GW004") // Nonexistent
	mustCode(t, diags, "GN001") // Take contract still applies
}

func TestEcoGnaUnknownMethodAndTypeGW004(t *testing.T) {
	reg := badRegistry(t, `methods:
  Handle.Cloze:
    params:
      - "![]byte"
    results:
      - "error"
types:
  Nope:
    fields:
      X: "!*Handle"
`)
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	_ = extlib.Echo("x")
}
`
	if got := countCode(ecoCheckWith(t, src, reg), "GW004"); got < 2 {
		t.Fatalf("expected GW004 for both Handle.Cloze and Nope, got %d", got)
	}
}

func TestEcoGnaCorrectFileNoValidationNoise(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	_ = extlib.Echo("x")
}
`
	diags := ecoCheck(t, src)
	mustNoCode(t, diags, "GW004")
	mustNoCode(t, diags, "GN003")
}

func TestEcoUnresolvedPackageNoValidationCrash(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte(`schema: 1
package: demo
functions:
  Take:
    params:
      - "!string"
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "demo"
func f() {
	demo.Take(nil)
}
`
	diags := ecoCheckWith(t, src, reg)
	mustNoCode(t, diags, "GW004") // unresolved package: applied unvalidated
	mustCode(t, diags, "GN001")   // contract still applied
}
