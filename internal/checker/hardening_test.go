package checker

import (
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// Architectural invariant (v1):
//
//	go/types determines what a symbol is;
//	.gna determines what nilability contract Gon assumes about it.
//
// These tests freeze call-shape and import-shape coverage that is easy to
// lose while the checker stays deliberately non-flow-sensitive.

func TestHardeningMethodExpressionGN001(t *testing.T) {
	// Method expression: Type.Method(recv, args...)
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("main.gna", []byte(`
schema: 1
package: main
methods:
  Writer.Write:
    params:
      - "![]byte"
    results:
      - "int"
      - "error"
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
type Writer interface {
	Write([]byte) (int, error)
}
func f(w Writer) {
	Writer.Write(w, nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("method expression: expected GN001, got %v", diags)
	}
}

func TestHardeningValueReceiverGN001(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("main.gna", []byte(`
schema: 1
package: main
methods:
  Box.Set:
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
type Box struct{}
func (b Box) Set(s string) {}
func f(b Box) {
	b.Set(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("value receiver: expected GN001, got %v", diags)
	}
}

func TestHardeningAliasedImportGN001(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("ext.gna", []byte(`
schema: 1
package: github.com/daniel-juvito/gon/internal/extlib
functions:
  Take:
    params:
      - "!string"
    results:
      - "error"
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import e "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	e.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("aliased import: expected GN001, got %v", diags)
	}
}

func TestHardeningPointerReceiverGN001(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("main.gna", []byte(`
schema: 1
package: main
methods:
  Box.Set:
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
type Box struct{}
func (b *Box) Set(s string) {}
func f(b *Box) {
	b.Set(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("pointer receiver: expected GN001, got %v", diags)
	}
}

func TestHardeningInterfaceMethodOrdinary(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("main.gna", []byte(`
schema: 1
package: main
methods:
  Writer.Write:
    params:
      - "[]byte"
    results:
      - "int"
      - "error"
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
type Writer interface {
	Write([]byte) (int, error)
}
func f(w Writer) {
	w.Write(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	for _, d := range diags {
		if d.Code == "GN001" {
			t.Fatalf("ordinary interface method must allow nil: %v", diags)
		}
	}
}

func TestHardeningInvariantTypesNotGna(t *testing.T) {
	// go/types still accepts the call; Gon only reports when .gna claims !T.
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, nil)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 0 {
		t.Fatalf("nil resolver must not invent contracts: %v", diags)
	}
}

func TestHardeningUnknownSymbolMessage(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte("schema: 1\npackage: demo\nfunctions:\n  Known:\n    params:\n      - \"int\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "demo"
func main() {
	demo.Unknown(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GW002" {
		t.Fatalf("expected GW002, got %v", diags)
	}
	if !strings.Contains(diags[0].Message, "demo.Unknown") {
		t.Fatalf("GW002 should name the symbol: %v", diags[0])
	}
}
