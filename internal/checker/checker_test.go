package checker

import (
	"go/token"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

func checkSource(t *testing.T, src string) []*Diagnostic {
	t.Helper()
	result := preproc.Process("test.gon", []byte(src))
	c, err := New("test.gon", result.Clean, result.NonNilOffsets)
	if err != nil {
		t.Fatalf("checker setup: %v", err)
	}
	return c.Check()
}

func TestGW001ParameterAndComparisonDirection(t *testing.T) {
	diags := checkSource(t, `package main
func f(x !*int) {
	if x == nil {}
	if x != nil {}
}`)

	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "GW001" || !strings.Contains(diags[0].Message, "always false") {
		t.Fatalf("unexpected == diagnostic: %#v", diags[0])
	}
	if diags[1].Code != "GW001" || !strings.Contains(diags[1].Message, "always true") {
		t.Fatalf("unexpected != diagnostic: %#v", diags[1])
	}
	if diags[0].Severity != SeverityWarning || diags[1].Severity != SeverityWarning {
		t.Fatalf("GW001 must be warnings")
	}
}

func TestGW001Receiver(t *testing.T) {
	diags := checkSource(t, `package main
type S struct{}
func (s !*S) f() {
	if s == nil {}
}`)
	if len(diags) != 1 || diags[0].Code != "GW001" {
		t.Fatalf("expected one GW001, got %v", diags)
	}
}

func TestShadowingNullableLocalDoesNotInheritNonNil(t *testing.T) {
	diags := checkSource(t, `package main
var x !*int
func f() {
	{
		var x *int
		if x == nil {}
	}
}`)
	if len(diags) != 0 {
		t.Fatalf("nullable shadow must not produce GW001: %v", diags)
	}
}

func TestShadowingNonNilLocalStillWarns(t *testing.T) {
	diags := checkSource(t, `package main
var x *int
func f() {
	{
		var x !*int
		if x == nil {}
	}
}`)
	if len(diags) != 1 || diags[0].Code != "GW001" {
		t.Fatalf("expected one GW001, got %v", diags)
	}
}

func TestGN001CoreCases(t *testing.T) {
	diags := checkSource(t, `package main
func f(x !*int) !*int {
	x = nil
	return nil
}
func g() {
	f(nil)
}`)
	if len(diags) != 3 {
		t.Fatalf("expected 3 GN001 diagnostics, got %d: %v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != "GN001" || d.Severity != SeverityError {
			t.Fatalf("unexpected diagnostic: %#v", d)
		}
	}
}

func TestGN002StructFields(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	A !*int
	B !*int
}
var s = S{A: nil}`)
	if len(diags) != 2 {
		t.Fatalf("expected GN001 + GN002, got %d: %v", len(diags), diags)
	}
	if diags[0].Code != "GN001" || diags[1].Code != "GN002" {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestNonLiteralAssignmentsAreAccepted(t *testing.T) {
	diags := checkSource(t, `package main
func get() *int { return nil }
var p *int
func f() {
	var a !*int = get()
	var b !*int = p
	_ = a
	_ = b
}`)
	if len(diags) != 0 {
		t.Fatalf("v1.0 must not perform flow analysis: %v", diags)
	}
}

func TestNoDiagnosticsForNullableNilComparison(t *testing.T) {
	diags := checkSource(t, `package main
func f(x *int) {
	if x == nil {}
	if x != nil {}
}`)
	if len(diags) != 0 {
		t.Fatalf("nullable values must not produce GW001: %v", diags)
	}
}

// Keep token imported in this package's test compile path as a sanity check
// that diagnostics use go/token positions.
var _ = token.NoPos

func TestGNAAnnotatedParamRejectsNil(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte(`
schema: 1
package: demo
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
import "demo"
func main() {
	demo.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("expected one GN001, got %v", diags)
	}
}

func TestGNAOrdinaryParamAllowsNil(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte(`
schema: 1
package: demo
functions:
  Take:
    params:
      - "string"
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
import "demo"
func main() {
	demo.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 0 {
		t.Fatalf("ordinary param must allow nil: %v", diags)
	}
}

func TestGNAMissingPackageNoWarning(t *testing.T) {
	reg := gna.NewRegistry()
	src := `package main
import "missing"
func main() {
	missing.F(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 0 {
		t.Fatalf("missing package must be ordinary with no diagnostic: %v", diags)
	}
}

func TestGNAUnknownSymbolWarning(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte(`
schema: 1
package: demo
functions:
  Known:
    params:
      - "int"
`))
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
}
