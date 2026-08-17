// Return-value contracts (v1.1 RFC).
//
// Executable specification for docs/rfc-return-value-contracts.md.
//
// Observable for "is non-nil source":
//   GW001 fires on `if x == nil` / `if x != nil` only when x is a known
//   non-nil source. That is the discriminant used by boundary tests.
//
// Positive cases that rely on annotated returns becoming non-nil sources
// are expected to FAIL under v1.0 and PASS once the feature lands.
// Regression cases must stay green throughout.
//
// Normative principle:
//   v1.1 introduces no new inference. It only promotes explicitly
//   annotated return positions to non-nil sources at their immediate
//   use sites.
package checker

import (
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

func checkWithGNA(t *testing.T, gnaSrc, gonSrc string) []*Diagnostic {
	t.Helper()
	reg := gna.NewRegistry()
	f, err := gna.LoadBytes("demo.gna", []byte(gnaSrc))
	if err != nil {
		t.Fatalf("gna load: %v", err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatalf("gna add: %v", err)
	}
	result := preproc.Process("test.gon", []byte(gonSrc))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatalf("checker setup: %v", err)
	}
	return c.Check()
}

func mustNoGN001(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Code == "GN001" {
			t.Fatalf("unexpected GN001: %v", diags)
		}
	}
}

func countGW001(diags []*Diagnostic) int {
	n := 0
	for _, d := range diags {
		if d.Code == "GW001" {
			n++
		}
	}
	return n
}

func mustHaveGW001(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if countGW001(diags) == 0 {
		t.Fatalf("expected at least one GW001 (non-nil source compared with nil), got %v", diags)
	}
}

func mustNotHaveGW001(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if countGW001(diags) != 0 {
		t.Fatalf("expected no GW001 (expression must be ordinary), got %v", diags)
	}
}

func TestRV_SingleAnnotatedReturnIsNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  MustLoad:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func f() {
	cfg := demo.MustLoad()
	if cfg == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

func TestRV_VarWithoutTypePromotesFromCall(t *testing.T) {
	// var cfg = demo.MustLoad() (no explicit type) must still become a
	// non-nil source via the return contract — same as short declaration.
	gnaSrc := `
schema: 1
package: demo
functions:
  MustLoad:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func f() {
	var cfg = demo.MustLoad()
	if cfg == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

func TestRV_SingleAnnotatedReturnIntoNonNilVar(t *testing.T) {
	// var c !*Config = call: binding is non-nil from explicit type (v1.0 path).
	// Still accepted; documents coexistence with return contracts.
	gnaSrc := `
schema: 1
package: demo
functions:
  MustLoad:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func f() {
	var c !*Config = demo.MustLoad()
	_ = c
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

func TestRV_SingleUnannotatedReturnIsOrdinary(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Load:
    results:
      - "*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func f() {
	c := demo.Load()
	if c == nil {}
	var x !*Config = c
	_ = x
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustNotHaveGW001(t, diags)
}

func TestRV_MultiReturnPositionalOnlyFirstIsNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Open:
    results:
      - "!*File"
      - "error"
`
	gonSrc := `package main
import "demo"
type File struct{}
func f() {
	f, err := demo.Open()
	if f == nil {}
	if err == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	if countGW001(diags) != 1 {
		t.Fatalf("expected exactly one GW001 (on f, not err), got %d: %v", countGW001(diags), diags)
	}
}

func TestRV_MultiReturnSecondPositionOrdinary(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Open:
    results:
      - "!*File"
      - "error"
`
	gonSrc := `package main
import "demo"
type File struct{}
func f() {
	_, err := demo.Open()
	if err == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustNotHaveGW001(t, diags)
}

func TestRV_BlankIdentifierKeepsContractOnKeptResult(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Open:
    results:
      - "!*File"
      - "error"
`
	gonSrc := `package main
import "demo"
type File struct{}
func f() {
	f, _ := demo.Open()
	if f == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

func TestRV_DirectReceiverFromAnnotatedResult(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  MustConfig:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func (c *Config) Reload() {}
func f() {
	demo.MustConfig().Reload()
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

func TestRV_LocalGonFunctionResultIsNonNilSource(t *testing.T) {
	gonSrc := `package main
func mustInt() !*int {
	n := 1
	return &n
}
func f() {
	x := mustInt()
	if x == nil {}
}
`
	diags := checkSource(t, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

func TestRV_MethodResultAnnotatedIsNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: main
methods:
  Factory.MustBuild:
    results:
      - "!*Product"
`
	gonSrc := `package main
type Factory struct{}
type Product struct{}
func (f *Factory) MustBuild() *Product { return nil }
func g(fac *Factory) {
	p := fac.MustBuild()
	if p == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustHaveGW001(t, diags)
}

func TestRV_ConversionDoesNotPropagateNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  MustLoad:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
type SpecialConfig Config
func f() {
	cfg := demo.MustLoad()
	if cfg == nil {}
	converted := (*SpecialConfig)(cfg)
	if converted == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	if countGW001(diags) != 1 {
		t.Fatalf("expected exactly one GW001 (on cfg only, not converted), got %d: %v",
			countGW001(diags), diags)
	}
}

func TestRV_TypeAssertionDoesNotPropagateNonNilSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  MustLoad:
    results:
      - "!*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
type Special interface{ M() }
func f() {
	cfg := demo.MustLoad()
	if cfg == nil {}
	s, _ := any(cfg).(Special)
	if s == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	if countGW001(diags) != 1 {
		t.Fatalf("expected exactly one GW001 (on cfg only, not assertion result), got %d: %v",
			countGW001(diags), diags)
	}
}

func TestRV_RegressionOrdinaryIntoNonNilStillAccepted(t *testing.T) {
	diags := checkSource(t, `package main
func get() *int { return nil }
var p *int
func f() {
	var a !*int = get()
	var b !*int = p
	_ = a
	_ = b
}
`)
	if len(diags) != 0 {
		t.Fatalf("v1.0 ordinary → !T must remain accepted: %v", diags)
	}
}

func TestRV_RegressionLiteralNilStillGN001(t *testing.T) {
	diags := checkSource(t, `package main
func f() !*int {
	return nil
}
`)
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("literal nil into !T result must stay GN001: %v", diags)
	}
}

func TestRV_RegressionUnannotatedReturnNotSource(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
functions:
  Load:
    results:
      - "*Config"
`
	gonSrc := `package main
import "demo"
type Config struct{}
func f() {
	c := demo.Load()
	if c == nil {}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
	mustNotHaveGW001(t, diags)
}
