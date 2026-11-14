package checker

import (
	"fmt"
	"strings"
	"testing"
)

// Diagnostics must not depend on Go's randomised map iteration order.
func TestDiagnosticOrderDeterministic(t *testing.T) {
	src := `package main
type T struct{}
type Conn struct {
	A !*T
	B !*T
	C !*T
	D !*T
	E !*T
	F !*T
}
func f() { _ = Conn{} }
var z Conn
`
	render := func() string {
		var b strings.Builder
		for _, d := range checkTyped(t, src) {
			fmt.Fprintf(&b, "%s:%d:%d:%s\n", d.Code, d.Line, d.Col, d.Message)
		}
		return b.String()
	}
	first := render()
	for i := 0; i < 40; i++ {
		if got := render(); got != first {
			t.Fatalf("run %d differs:\n--- first ---\n%s--- got ---\n%s", i, first, got)
		}
	}
}

// Local struct fields are reported in declaration order.
func TestMissingFieldsReportedInDeclarationOrder(t *testing.T) {
	d := checkTyped(t, `package main
type T struct{}
type Conn struct {
	Zeta  !*T
	Alpha !*T
	Mid   !*T
}
func f() { _ = Conn{} }
`)
	var got []string
	for _, x := range d {
		if x.Code == "GN002" {
			got = append(got, x.Message)
		}
	}
	want := []string{
		"missing required non-nil field Conn.Zeta",
		"missing required non-nil field Conn.Alpha",
		"missing required non-nil field Conn.Mid",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("order:\n got %v\nwant %v", got, want)
	}
}
