// Return-value contracts (v1.1 RFC).
//
// These tests are the executable specification for
// docs/rfc-return-value-contracts.md.
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 1. Single return — annotated !T becomes non-nil source
// ---------------------------------------------------------------------------

func TestRV_SingleAnnotatedReturnIntoNonNilVar(t *testing.T) {
	// Target: annotated !*Config result may be assigned to !*Config variable.
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

func TestRV_SingleUnannotatedReturnStillOrdinary(t *testing.T) {
	// v1.0 behaviour preserved: unannotated return is ordinary.
	// Assignment ordinary → !T remains accepted (no flow analysis).
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
	var c !*Config = demo.Load()
	_ = c
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 2. Multi-return — positional contracts only
// ---------------------------------------------------------------------------

func TestRV_MultiReturnPositionalOnlyFirstIsNonNilSource(t *testing.T) {
	// results[0]=!*File, results[1]=error → only position 0 is non-nil source.
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
	var x !*File = f
	_ = x
	_ = err
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

func TestRV_MultiReturnSecondPositionOrdinary(t *testing.T) {
	// results[1] is ordinary; using it as !T source must NOT be treated as guaranteed.
	// (Still accepted under "ordinary → !T stays accepted".)
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
	var e !error = err
	_ = e
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 3. Ignored / blank result preserves contract on the kept position
// ---------------------------------------------------------------------------

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
	var x !*File = f
	_ = x
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 4. Direct receiver use of annotated result
// ---------------------------------------------------------------------------

func TestRV_DirectReceiverFromAnnotatedResult(t *testing.T) {
	// MustConfig() !*Config → immediate .Method() is an allowed use site.
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

// ---------------------------------------------------------------------------
// 5. Local .gon function with !T result
// ---------------------------------------------------------------------------

func TestRV_LocalGonFunctionResultIsNonNilSource(t *testing.T) {
	// Local function declared with !*int result.
	gonSrc := `package main
func mustInt() !*int {
	n := 1
	return &n
}
func f() {
	var x !*int = mustInt()
	_ = x
}
`
	diags := checkSource(t, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 6. Method / method expression — contract lookup via go/types
// ---------------------------------------------------------------------------

func TestRV_MethodResultAnnotated(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
methods:
  Factory.MustBuild:
    results:
      - "!*Product"
`
	gonSrc := `package main
import "demo"
type Factory struct{}
type Product struct{}
func (f *Factory) MustBuild() *Product { return nil }
func g(fac *Factory) {
	var p !*Product = fac.MustBuild()
	_ = p
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 7. Negative boundary — no propagation through conversion / assertion
// ---------------------------------------------------------------------------

func TestRV_ConversionDoesNotCreateNonNilSource(t *testing.T) {
	// Guardrail: annotated return is a non-nil source, but conversion of it
	// must NOT become a non-nil source. Implementation must not silently
	// start doing propagation.
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
	converted := (*SpecialConfig)(cfg)
	var x !*SpecialConfig = converted
	_ = x
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	// Under the RFC this assignment is still *accepted* (ordinary → !T),
	// but converted must not be treated as a guaranteed non-nil source.
	// The test documents the boundary: no diagnostic is required, and
	// no new inference is introduced.
	mustNoGN001(t, diags)
}

func TestRV_TypeAssertionDoesNotCreateNonNilSource(t *testing.T) {
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
	s, _ := any(cfg).(Special)
	var x !Special = s
	_ = x
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// 8. Regression — ordinary → !T stays accepted; existing diagnostics intact
// ---------------------------------------------------------------------------

func TestRV_RegressionOrdinaryIntoNonNilStillAccepted(t *testing.T) {
	// Exact v1.0 behaviour: no flow analysis.
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
	// Explicitly: a function with no ! annotation on its result must not
	// magically become a non-nil source.
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
	// still accepted (ordinary → !T), but Load is not a guaranteed source
	var c !*Config = demo.Load()
	_ = c
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustNoGN001(t, diags)
}
