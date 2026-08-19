package fmt

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatPreservesBang(t *testing.T) {
	out, err := Format("t.gon", []byte("package main\nfunc f(x !*int) {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "!*int") {
		t.Fatalf("expected !*int, got:\n%s", out)
	}
}

func TestFormatIdempotentBang(t *testing.T) {
	src := []byte("package main\nfunc f(x !*int) {}\n")
	out1, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := Format("t.gon", out1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), "!*int") {
		t.Fatalf("second format lost !: %s", out2)
	}
}

// TestFormatBangPosition verifies ! is immediately before the type token,
// not somewhere earlier in an identifier that merely contains the snippet.
func TestFormatBangPosition(t *testing.T) {
	src := []byte("package main\nfunc f(x !*int) {}\n")
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("!*int")) {
		t.Fatalf("missing !*int:\n%s", out)
	}
	// Must not produce corrupted forms like !func or f(!x
	if bytes.Contains(out, []byte("!func")) || bytes.Contains(out, []byte("(!x")) {
		t.Fatalf("! inserted at wrong site:\n%s", out)
	}
}

// TestFormatShortTypeNameDoesNotMatchInsideIdent is the regression for
// the substring-match bug: snippet "S" must not latch onto the leading
// "S" of "Something".
func TestFormatShortTypeNameDoesNotMatchInsideIdent(t *testing.T) {
	src := []byte(`package main
type S struct{}
func Something() {}
func f(x !S) { _ = Something() }
`)
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "!S") {
		t.Fatalf("expected !S on the type, got:\n%s", out)
	}
	if strings.Contains(s, "!Something") {
		t.Fatalf("! latched onto Something:\n%s", out)
	}
	// Exact param form after format
	if !strings.Contains(s, "func f(x !S)") && !strings.Contains(s, "func f(x !S) {") {
		// go/format may put brace on same or next line; require the param site.
		if !strings.Contains(s, "(x !S)") {
			t.Fatalf("!S not at param site:\n%s", out)
		}
	}
}

// TestFormatIntSnippetDoesNotMatchInsidePrint guards "int" vs "print".
func TestFormatIntSnippetDoesNotMatchInsidePrint(t *testing.T) {
	src := []byte(`package main
import "fmt"
func f(x !*int) { fmt.Println(x) }
`)
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "!*int") {
		t.Fatalf("expected !*int, got:\n%s", out)
	}
	if strings.Contains(s, "!Println") || strings.Contains(s, "Pr!int") || strings.Contains(s, "!intln") {
		t.Fatalf("! corrupted Println:\n%s", out)
	}
}

func TestFormatMultipleSameType(t *testing.T) {
	src := []byte("package main\nfunc f(a !*int, b !*int) {}\n")
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	if c := strings.Count(string(out), "!*int"); c != 2 {
		t.Fatalf("expected 2 !*int, got %d in:\n%s", c, out)
	}
}

func TestFormatStructFieldAndFunc(t *testing.T) {
	src := []byte(`package main
type T struct {
	Client !*int
}
func f(x !T) {}
`)
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "Client !*int") && !strings.Contains(s, "Client  !*int") {
		// allow minor spacing variance but require bang before *int near Client
		if !strings.Contains(s, "!*int") {
			t.Fatalf("missing field !:\n%s", out)
		}
	}
	if !strings.Contains(s, "!T") {
		t.Fatalf("missing param !T:\n%s", out)
	}
	if strings.Contains(s, "!type") || strings.Contains(s, "!struct") {
		t.Fatalf("! at wrong keyword:\n%s", out)
	}
}

func TestFormatNoBangUnchanged(t *testing.T) {
	src := []byte("package main\nfunc f(x *int) {}\n")
	out, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "!") {
		t.Fatalf("unexpected !:\n%s", out)
	}
}

func TestIndexWordBoundary(t *testing.T) {
	cases := []struct {
		hay, snip string
		want      int
	}{
		{"func f(x S)", "S", 9},
		{"Something", "S", -1}, // would be prefix of ident
		{"(x S)", "S", 3},
		{"print", "int", -1},
		{"*int)", "int", 1},
		{"*int)", "*int", 0},
		{"func f(x *int, y *int)", "*int", 9},
	}
	for _, tc := range cases {
		got := indexWordBoundary([]byte(tc.hay), []byte(tc.snip))
		if got != tc.want {
			t.Fatalf("indexWordBoundary(%q, %q)=%d want %d", tc.hay, tc.snip, got, tc.want)
		}
	}
}
