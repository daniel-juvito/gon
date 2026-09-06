package fmt

import (
	"bytes"
	"strings"
	"testing"
)

func mustFormat(t *testing.T, src string) string {
	t.Helper()
	out, err := Format("t.gon", []byte(src))
	if err != nil {
		t.Fatalf("Format: %v\nsrc:\n%s", err, src)
	}
	return string(out)
}

func TestFormatPreservesBang(t *testing.T) {
	out := mustFormat(t, "package main\nfunc f(x !*int) {}\n")
	if !strings.Contains(out, "!*int") {
		t.Fatalf("expected !*int, got:\n%s", out)
	}
}

func TestFormatPreservesElementBang(t *testing.T) {
	// v1.7 / M2b: element `!` must survive the strip / go-format / re-insert
	// round-trip in place.
	src := "package main\n" +
		"func a() []!*T { return nil }\n" +
		"func b(m map[string]!*T) {}\n" +
		"func c() [3]!*T { return [3]!*T{} }\n"
	out := mustFormat(t, src)
	for _, want := range []string{"[]!*T", "map[string]!*T", "[3]!*T"} {
		if !strings.Contains(out, want) {
			t.Fatalf("element ! not preserved (want %q):\n%s", want, out)
		}
	}
	if out != mustFormat(t, out) {
		t.Fatalf("not idempotent:\n%s", out)
	}
}

func TestFormatIdempotentBang(t *testing.T) {
	src := "package main\nfunc f(x !*int) {}\n"
	out1 := mustFormat(t, src)
	out2 := mustFormat(t, out1)
	if out1 != out2 {
		t.Fatalf("not idempotent:\n--- 1 ---\n%s\n--- 2 ---\n%s", out1, out2)
	}
	if !strings.Contains(out2, "!*int") {
		t.Fatalf("second format lost !: %s", out2)
	}
}

func TestFormatBangPosition(t *testing.T) {
	out := mustFormat(t, "package main\nfunc f(x !*int) {}\n")
	if !bytes.Contains([]byte(out), []byte("!*int")) {
		t.Fatalf("missing !*int:\n%s", out)
	}
	if strings.Contains(out, "!func") || strings.Contains(out, "(!x") {
		t.Fatalf("! inserted at wrong site:\n%s", out)
	}
}

func TestFormatShortTypeNameDoesNotMatchInsideIdent(t *testing.T) {
	out := mustFormat(t, `package main
type S struct{}
func Something() {}
func f(x !S) { _ = Something() }
`)
	if !strings.Contains(out, "(x !S)") {
		t.Fatalf("expected !S at param site, got:\n%s", out)
	}
	if strings.Contains(out, "!Something") {
		t.Fatalf("! latched onto Something:\n%s", out)
	}
}

func TestFormatIntSnippetDoesNotMatchInsidePrint(t *testing.T) {
	out := mustFormat(t, `package main
import "fmt"
func f(x !*int) { fmt.Println(x) }
`)
	if !strings.Contains(out, "!*int") {
		t.Fatalf("expected !*int, got:\n%s", out)
	}
	if strings.Contains(out, "!Println") || strings.Contains(out, "Pr!int") || strings.Contains(out, "!intln") {
		t.Fatalf("! corrupted Println:\n%s", out)
	}
}

func TestFormatMultipleSameType(t *testing.T) {
	out := mustFormat(t, "package main\nfunc f(a !*int, b !*int) {}\n")
	if c := strings.Count(out, "!*int"); c != 2 {
		t.Fatalf("expected 2 !*int, got %d in:\n%s", c, out)
	}
}

func TestFormatStructFieldAndFunc(t *testing.T) {
	out := mustFormat(t, `package main
type T struct {
	Client !*int
}
func f(x !T) {}
`)
	if !strings.Contains(out, "!*int") {
		t.Fatalf("missing field !:\n%s", out)
	}
	if !strings.Contains(out, "!T") {
		t.Fatalf("missing param !T:\n%s", out)
	}
	if strings.Contains(out, "!type") || strings.Contains(out, "!struct") {
		t.Fatalf("! at wrong keyword:\n%s", out)
	}
}

func TestFormatNoBangUnchanged(t *testing.T) {
	out := mustFormat(t, "package main\nfunc f(x *int) {}\n")
	if strings.Contains(out, "!") {
		t.Fatalf("unexpected !:\n%s", out)
	}
}

// Regression (v1.4.1): a type used as BOTH !T and T in the same file. The old
// "nth textual match" heuristic mis-placed every ! here.
func TestFormatMixedBangAndPlainSameType(t *testing.T) {
	out := mustFormat(t, `package main
func get() *string { return nil }
func f() {
	var x !*string = get() // ! on the var, plain on the return
	_ = x
}
`)
	if !strings.Contains(out, "func get() *string") {
		t.Fatalf("return type must stay plain *string:\n%s", out)
	}
	if !strings.Contains(out, "var x !*string = get()") {
		t.Fatalf("var type must stay !*string:\n%s", out)
	}
	if strings.Contains(out, "get() !*string") || strings.Contains(out, "var x *string =") {
		t.Fatalf("! mis-aligned:\n%s", out)
	}
}

func TestFormatInterfaceMixedBangAndPlain(t *testing.T) {
	out := mustFormat(t, `package main
type Reader interface{ Read() }
type ReadCloser interface {
	Reader
	Close()
}
func mk() !Reader { return nil }
func ordinary() Reader { return nil }
func f() {
	r := ordinary()
	var a !Reader = r
	var rc !ReadCloser = mk()
	_ = a
	_ = rc
}
`)
	for _, want := range []string{
		"type Reader interface",     // decl: plain
		"\tReader\n",                // embedded: plain
		"func mk() !Reader",         // result: !
		"func ordinary() Reader",    // result: plain
		"var a !Reader = r",         // var: !
		"var rc !ReadCloser = mk()", // var: !
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "!!") || strings.Contains(out, "type !Reader") || strings.Contains(out, "ordinary() !Reader") {
		t.Fatalf("! corrupted:\n%s", out)
	}
}

// go/format adds a trailing comma when it breaks a composite literal onto
// multiple lines; the token walker must resync past it.
func TestFormatTrailingCommaResync(t *testing.T) {
	out := mustFormat(t, `package main
type P struct{ A, B *int }
func f() {
	n := 1
	_ = []*P{{A: &n, B: &n}, {A: &n, B: &n}}
	var x !*int = &n
	_ = x
}
`)
	if !strings.Contains(out, "var x !*int = &n") {
		t.Fatalf("! lost after composite literal:\n%s", out)
	}
	if strings.Contains(out, "!*P") || strings.Contains(out, "[]*!P") {
		t.Fatalf("! leaked into the slice type:\n%s", out)
	}
}

// A file go/format leaves byte-identical must round-trip unchanged.
func TestFormatIdempotentOnMixedFile(t *testing.T) {
	src := `package main

type Reader interface{ Read() }

func mk() !Reader { return nil }

func f() {
	var a !Reader = mk()
	_ = a
}
`
	out1 := mustFormat(t, src)
	out2 := mustFormat(t, out1)
	if out1 != out2 {
		t.Fatalf("not idempotent:\n--- 1 ---\n%s--- 2 ---\n%s", out1, out2)
	}
}
