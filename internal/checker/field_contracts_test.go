// Field contracts (v1.2 RFC).
//
// Executable specification for docs/rfc-field-contracts.md.
//
// Normative anchors:
//   A !T field is an invariant of its declared storage location, not a
//   one-time property of its construction.
//
//   GN002 recursively inspects types according to zero-value containment.
//   A nested type is traversed when the containing type's zero value contains
//   an actual value of that nested type. Traversal stops at indirection
//   boundaries and never follows reachable objects.
//
//   Gon validates contracts locally at construction, mutation, and use sites;
//   it does not reconstruct object state from control flow.
//
// Observable discriminants:
//   GN002  — construction leaves a ! field at zero value
//   GN001  — explicit/known nil assigned into a ! field
//   GW001  — selector of a ! field used in nil comparison (non-nil source)
//
// Positive cases that rely on new semantics are expected to FAIL under v1.1
// and PASS once the feature lands. All existing v1.1 tests must stay green.
package checker

import (
	"strings"
	"testing"
)

func countCode(diags []*Diagnostic, code string) int {
	n := 0
	for _, d := range diags {
		if d.Code == code {
			n++
		}
	}
	return n
}

func mustHaveGN002(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if countCode(diags, "GN002") == 0 {
		t.Fatalf("expected at least one GN002, got %v", diags)
	}
}

func mustHaveGN001(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if countCode(diags, "GN001") == 0 {
		t.Fatalf("expected at least one GN001, got %v", diags)
	}
}

func mustNoGN002(t *testing.T, diags []*Diagnostic) {
	t.Helper()
	if countCode(diags, "GN002") != 0 {
		t.Fatalf("expected no GN002, got %v", diags)
	}
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
// Construction sites → GN002
// ---------------------------------------------------------------------------

func TestFC_DirectFieldMissingInComposite(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
var _ = S{}
`)
	mustHaveGN002(t, diags)
}

func TestFC_DirectFieldProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(c *int) {
	_ = S{Client: c}
}
`)
	mustNoGN002(t, diags)
	mustNoGN001(t, diags)
}

func TestFC_DirectFieldNilInCompositeGN001(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
var _ = S{Client: nil}
`)
	mustHaveGN001(t, diags)
}

func TestFC_DeclareThenPopulateGN002(t *testing.T) {
	// var s S is a construction site that produces the zero value.
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f() {
	var s S
	_ = s
}
`)
	mustHaveGN002(t, diags)
}

func TestFC_VarWithZeroCompositeGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f() {
	var s S = S{}
	_ = s
}
`)
	mustHaveGN002(t, diags)
}

func TestFC_VarWithFullCompositeOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(c *int) {
	var s S = S{Client: c}
	_ = s
}
`)
	mustNoGN002(t, diags)
	mustNoGN001(t, diags)
}

func TestFC_AnonymousStructTraversed(t *testing.T) {
	diags := checkSource(t, `package main
func f() {
	_ = struct{ Client !*int }{}
}
`)
	mustHaveGN002(t, diags)
}

func TestFC_AnonymousStructProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
func f(c *int) {
	_ = struct{ Client !*int }{Client: c}
}
`)
	mustNoGN002(t, diags)
	mustNoGN001(t, diags)
}

func TestFC_EmbeddedStructTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Outer struct {
	Inner
}
var _ = Outer{}
`)
	mustHaveGN002(t, diags)
}

func TestFC_EmbeddedStructProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Outer struct {
	Inner
}
func f(c *int) {
	_ = Outer{Inner: Inner{Client: c}}
}
`)
	mustNoGN002(t, diags)
	mustNoGN001(t, diags)
}

func TestFC_ArrayOfStructTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Arrayed struct {
	Items [2]Inner
}
var _ = Arrayed{}
`)
	mustHaveGN002(t, diags)
}

func TestFC_ArrayOfStructProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Arrayed struct {
	Items [2]Inner
}
func f(c *int) {
	_ = Arrayed{Items: [2]Inner{
		{Client: c},
		{Client: c},
	}}
}
`)
	mustNoGN002(t, diags)
	mustNoGN001(t, diags)
}

func TestFC_NestedArrayTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
var _ = [2][1]Inner{}
`)
	mustHaveGN002(t, diags)
}

// ---------------------------------------------------------------------------
// Indirection boundaries — must NOT traverse
// ---------------------------------------------------------------------------

func TestFC_PointerFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Box struct {
	Ptr *Inner
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

func TestFC_SliceFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Box struct {
	Slice []Inner
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

func TestFC_MapFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Box struct {
	M map[string]Inner
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

func TestFC_InterfaceFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Box struct {
	I interface{}
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

func TestFC_ChanFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Box struct {
	C chan Inner
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

func TestFC_FuncFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Box struct {
	F func() !*int
}
var _ = Box{}
`)
	mustNoGN002(t, diags)
}

// ---------------------------------------------------------------------------
// Non-construction sites — must NOT GN002
// ---------------------------------------------------------------------------

func TestFC_AssignmentFromExistingNotConstruction(t *testing.T) {
	// b := a is a copy of an existing value, not a construction site.
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(a S) {
	b := a
	_ = b
}
`)
	mustNoGN002(t, diags)
}

func TestFC_RangeLoopCopyNotConstruction(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(items []S) {
	for _, v := range items {
		_ = v
	}
}
`)
	mustNoGN002(t, diags)
}

// ---------------------------------------------------------------------------
// Mutation sites → GN001
// ---------------------------------------------------------------------------

func TestFC_MutationExplicitNilGN001(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(s *S) {
	s.Client = nil
}
`)
	mustHaveGN001(t, diags)
}

func TestFC_MutationOrdinaryAccepted(t *testing.T) {
	// Conservative rule: ordinary value into !T field is accepted.
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(s *S, ordinary *int) {
	s.Client = ordinary
}
`)
	mustNoGN001(t, diags)
}

func TestFC_MutationNonNilSourceOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func must() !*int {
	n := 1
	return &n
}
func f(s *S) {
	s.Client = must()
}
`)
	mustNoGN001(t, diags)
}

// ---------------------------------------------------------------------------
// Selector use → non-nil source
// ---------------------------------------------------------------------------

func TestFC_SelectorIsNonNilSource(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(s S) {
	if s.Client == nil {}
}
`)
	mustHaveGW001(t, diags)
}

func TestFC_SelectorAssignedToNonNilOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func f(s S) {
	var x !*int = s.Client
	_ = x
}
`)
	mustNoGN001(t, diags)
}

func TestFC_SelectorAsArgToNonNilParamOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
func take(p !*int) {}
func f(s S) {
	take(s.Client)
}
`)
	mustNoGN001(t, diags)
}

func TestFC_SelectorDoesNotPropagateThroughConversion(t *testing.T) {
	// Consistent with v1.1: conversion does not propagate non-nil source.
	diags := checkSource(t, `package main
type S struct {
	Client !*int
}
type Alias *int
func f(s S) {
	a := Alias(s.Client)
	if a == nil {}
}
`)
	mustNotHaveGW001(t, diags)
}

// ---------------------------------------------------------------------------
// Cross-package via .gna types:
// ---------------------------------------------------------------------------

func TestFC_GNATypeFieldConstructionGN002(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
types:
  Config:
    fields:
      Client: "!*http.Client"
      Log:    "!*log.Logger"
`
	gonSrc := `package main
import "demo"
func f() {
	_ = demo.Config{}
}
`
	diags := checkWithGNA(t, gnaSrc, gonSrc)
	mustHaveGN002(t, diags)
}

func TestFC_GNATypeFieldProvidedOK(t *testing.T) {
	gnaSrc := `
schema: 1
package: demo
types:
  Config:
    fields:
      Client: "!*http.Client"
`
	gonSrc := `package main
import "demo"
func f(c *http.Client) {
	_ = demo.Config{Client: c}
}
`
	// Note: http may not resolve in unit test; we only care that the
	// field contract is satisfied when Client is provided. If type-check
	// fails the checker still runs local/gna logic where possible.
	_ = gnaSrc
	_ = gonSrc
	// Full cross-package resolution tested in e2e; here we document the
	// expected contract shape. Implementation must support types: in .gna.
	t.Skip("cross-package .gna types: requires full package load; covered by e2e")
}

// ---------------------------------------------------------------------------
// Partial composite still GN002 for missing fields
// ---------------------------------------------------------------------------

func TestFC_PartialCompositeStillGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct {
	A !*int
	B !*int
}
var _ = S{A: new(int)}
`)
	mustHaveGN002(t, diags)
	// B is missing → GN002; A is provided non-nil → no GN001 for A.
	if countCode(diags, "GN002") < 1 {
		t.Fatalf("expected GN002 for missing B: %v", diags)
	}
}

// ---------------------------------------------------------------------------
// Diagnostic quality: field path in message
// ---------------------------------------------------------------------------

func TestFC_GN002MessageIncludesFieldPath(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct {
	Client !*int
}
type Outer struct {
	Inner
}
var _ = Outer{}
`)
	mustHaveGN002(t, diags)
	found := false
	for _, d := range diags {
		if d.Code == "GN002" && (strings.Contains(d.Message, "Client") || strings.Contains(d.Message, "Inner")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GN002 message should include field path, got %v", diags)
	}
}
