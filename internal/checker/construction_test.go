package checker

import (
	"strings"
	"testing"
)

func hasCode(diags []*Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestM2a_KeyedCompleteOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B *int }
func f(a *int) { _ = S{A: a, B: nil} }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("unexpected: %v", diags)
	}
}

func TestM2a_KeyedMissingGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int }
var _ = S{}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_KeyedExplicitNilGN001(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int }
func f() { _ = S{A: nil} }
`)
	fcMustHaveGN001(t, diags)
}

func TestM2a_UnkeyedCompleteOK(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B *int }
func f(a *int) { _ = S{a, nil} }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("unexpected: %v", diags)
	}
}

func TestM2a_UnkeyedMissingGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B !*int }
func f(a *int) { _ = S{a} }
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_UnkeyedExplicitNilGN001(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B *int }
func f() { _ = S{nil, nil} }
`)
	fcMustHaveGN001(t, diags)
}

func TestM2a_UnkeyedPositionalOrder(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B !*int }
func f(a *int) { _ = S{a} }
`)
	fcMustHaveGN002(t, diags)
	for _, d := range diags {
		if d.Code == "GN002" && strings.Contains(d.Message, "A") && !strings.Contains(d.Message, "B") {
			t.Fatalf("should not report missing A: %v", diags)
		}
	}
	foundB := false
	for _, d := range diags {
		if d.Code == "GN002" && strings.Contains(d.Message, "B") {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("expected GN002 for B: %v", diags)
	}
}

func TestM2a_NewLocalStructGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { Client !*int }
func f() { _ = new(S) }
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_NewNestedGN002(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { DB !*int }
type Outer struct { Inner Inner }
func f() { _ = new(Outer) }
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_NewNonStructSilent(t *testing.T) {
	diags := checkSource(t, `package main
func f() { _ = new(int); _ = new(*int) }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("new(non-struct) must be silent: %v", diags)
	}
}

func TestM2a_PackageGlobalNewGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { Client !*int }
var p = new(S)
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_NestedStructKeyed(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { X !*int }
type Outer struct { Inner Inner }
var _ = Outer{}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_NestedStructProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { X !*int }
type Outer struct { Inner Inner }
func f(x *int) { _ = Outer{Inner: Inner{X: x}} }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("unexpected: %v", diags)
	}
}

func TestM2a_EmbeddedStructTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { Client !*int }
type Outer struct { Inner }
var _ = Outer{}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_EmbeddedProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { Client !*int }
type Outer struct { Inner }
func f(c *int) { _ = Outer{Inner: Inner{Client: c}} }
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("unexpected GN002: %v", diags)
	}
}

func TestM2a_AnonymousStructTraversed(t *testing.T) {
	diags := checkSource(t, `package main
var _ = struct{ X !*int }{}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_AnonymousStructProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
func f(x *int) { _ = struct{ X !*int }{X: x} }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("unexpected: %v", diags)
	}
}

func TestM2a_UnkeyedEmbeddedPositional(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { Client !*int }
type Outer struct { Inner; Y *int }
func f(c *int) { _ = Outer{Inner{Client: c}, nil} }
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("embedded provided positionally should be OK: %v", diags)
	}
}

func TestM2a_PointerFieldNotTraversed(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { Client !*int }
type Outer struct { P *Inner }
var _ = Outer{}
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("must not walk through pointer: %v", diags)
	}
}

func TestM2a_TraceAttachedOnGN002(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { Client !*int }
var _ = S{}
`)
	fcMustHaveGN002(t, diags)
	var d *Diagnostic
	for _, x := range diags {
		if x.Code == "GN002" {
			d = x
			break
		}
	}
	if d.Trace == nil {
		t.Fatalf("GN002 must carry ContractTrace: %#v", d)
	}
	if d.Trace.Origin == "" {
		t.Fatalf("Trace.Origin should be set: %#v", d.Trace)
	}
}

func TestM2a_TraceNestedPath(t *testing.T) {
	diags := checkSource(t, `package main
type Inner struct { DB !*int }
type Outer struct { Inner Inner }
var _ = Outer{}
`)
	fcMustHaveGN002(t, diags)
	found := false
	for _, d := range diags {
		if d.Code != "GN002" || d.Trace == nil {
			continue
		}
		if strings.Contains(d.Trace.FieldPath(), "DB") || strings.Contains(d.Message, "DB") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected nested trace/message with DB: %v", diags)
	}
}

func TestM2a_TraceOnNew(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { X !*int }
func f() { _ = new(S) }
`)
	fcMustHaveGN002(t, diags)
	for _, d := range diags {
		if d.Code == "GN002" && d.Trace != nil && strings.Contains(d.Trace.Origin, "new") {
			return
		}
	}
	t.Fatalf("expected traced GN002 for new(S): %v", diags)
}

func TestM2a_TraceDoesNotChangeCodeOrSeverity(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int }
var _ = S{}
`)
	for _, d := range diags {
		if d.Code == "GN002" {
			if d.Severity != SeverityError {
				t.Fatalf("GN002 must remain error: %#v", d)
			}
			if !strings.Contains(d.String(), "error GN002:") {
				t.Fatalf("String() wire format regress: %q", d.String())
			}
		}
	}
}

func TestM2a_GW003_DeclaredNonNilAssert(t *testing.T) {
	diags := checkSource(t, `package main
func f(x !*int) { _ = x.(*int) }
`)
	if !hasCode(diags, "GW003") {
		t.Fatalf("expected GW003, got %v", diags)
	}
}

func TestM2a_GW003_NullableNoWarning(t *testing.T) {
	diags := checkSource(t, `package main
func f(x *int) { _ = x.(*int) }
`)
	if hasCode(diags, "GW003") {
		t.Fatalf("nullable must not warn: %v", diags)
	}
}

func TestM2a_GW003_NoFlowPromotion(t *testing.T) {
	diags := checkSource(t, `package main
func f(x *int) {
	if x != nil { _ = x.(*int) }
}
`)
	if hasCode(diags, "GW003") {
		t.Fatalf("flow must not produce GW003: %v", diags)
	}
}

func TestM2a_GW003_NoConversionPropagation(t *testing.T) {
	diags := checkSource(t, `package main
func f(x !*int) {
	var y *int = x
	_ = y.(*int)
}
`)
	if hasCode(diags, "GW003") {
		t.Fatalf("conversion must not produce GW003: %v", diags)
	}
}

func TestM2a_GW003_SelectorNotDeclaration(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { X !*int }
func f(s S) { _ = s.X.(*int) }
`)
	if hasCode(diags, "GW003") {
		t.Fatalf("selector assertion out of GW003 scope: %v", diags)
	}
}

func TestM2a_MapLiteralSilent(t *testing.T) {
	diags := checkSource(t, `package main
func f() { _ = map[string]*int{} }
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("map is not M2a: %v", diags)
	}
}

func TestM2a_SliceLiteralSilent(t *testing.T) {
	diags := checkSource(t, `package main
func f() { _ = []*int{} }
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("slice is not M2a: %v", diags)
	}
}

func TestM2a_InterfaceNilSilent(t *testing.T) {
	diags := checkSource(t, `package main
func f() { var _ interface{} = nil; var _ error = nil }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("interface nil is not M2a: %v", diags)
	}
}

func TestM2a_Regression_FC_DirectFieldMissing(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { Client !*int }
var _ = S{}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_Regression_FC_PartialComposite(t *testing.T) {
	diags := checkSource(t, `package main
type S struct { A !*int; B !*int }
var _ = S{A: new(int)}
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_FixedArrayOfStructMissingGN002(t *testing.T) {
	diags := checkSource(t, `package main
type Cell struct { X !*int }
func f() { _ = [2]Cell{} }
`)
	fcMustHaveGN002(t, diags)
}

func TestM2a_FixedArrayProvidedOK(t *testing.T) {
	diags := checkSource(t, `package main
type Cell struct { X !*int }
func f(x *int) { _ = [1]Cell{{X: x}} }
`)
	if hasCode(diags, "GN002") || hasCode(diags, "GN001") {
		t.Fatalf("unexpected: %v", diags)
	}
}

func TestM2a_SliceOfStructSilent(t *testing.T) {
	diags := checkSource(t, `package main
type Cell struct { X !*int }
func f() { _ = []Cell{} }
`)
	if hasCode(diags, "GN002") {
		t.Fatalf("slice must not trigger GN002: %v", diags)
	}
}
