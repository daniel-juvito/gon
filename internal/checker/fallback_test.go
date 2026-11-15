package checker

import (
	"go/token"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// typeCheckFallback is what runs when go/packages cannot load the file (no
// enclosing module); it type-checks with the standalone importer. It was never
// exercised by a test, and it is the path the -trimpath GOROOT bug broke.
func TestTypeCheckFallbackResolvesStdlibSelectors(t *testing.T) {
	src := "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"x\") }\n"
	r := preproc.Process("t.gon", []byte(src))
	c, err := New("t.gon", r.Clean, r.NonNilOffsets)
	if err != nil {
		t.Fatal(err)
	}
	c.typeCheckFallback()
	if c.info == nil {
		t.Fatal("fallback produced no type information")
	}
	if len(c.info.Selections) == 0 && len(c.info.Uses) == 0 {
		t.Fatal("fallback recorded nothing for fmt.Println")
	}
}

func TestTypeCheckFallbackWithoutSourceIsNoOp(t *testing.T) {
	c := &Checker{fset: token.NewFileSet()}
	c.typeCheckFallback() // no file, no source: must not panic
	if c.info != nil {
		t.Fatal("expected no type information")
	}
}

func TestDiagnosticStringAndIsError(t *testing.T) {
	e := &Diagnostic{Severity: SeverityError, Code: "GN001", Message: "m", File: "f.gon", Line: 3, Col: 4}
	w := &Diagnostic{Severity: SeverityWarning, Code: "GW001", Message: "w", File: "f.gon", Line: 1, Col: 2}
	if !e.IsError() || w.IsError() {
		t.Fatalf("IsError: error=%v warning=%v", e.IsError(), w.IsError())
	}
	if got := e.String(); got != "f.gon:3:4: error GN001: m" {
		t.Fatalf("String() = %q", got)
	}
	if got := w.String(); got != "f.gon:1:2: warning GW001: w" {
		t.Fatalf("String() = %q", got)
	}
}
