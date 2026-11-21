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

// v1.8 / M3: `!` survives the format round-trip in every generic position
// (constraint-typed parameter, instantiated field, generic result), with
// formatting normalised by go/format.
func TestFormatPreservesBangInGenerics(t *testing.T) {
	src := `package main

type T struct{}

type Box[X any] struct {
	v !*X
}

type Reg[K comparable, V any] struct {
	items map[K]!*V
}

func Ptr[P ~*T](p !P) {}

func NN[X any]() !*X { return new(X) }

func Must[V any](p *V) !*V { return p }

func f() {
	_ = Box[T]{v: &T{}}
	_ = []!*T{&T{}}
}
`
	out := mustFormat(t, src)
	if out != src {
		t.Fatalf("round-trip changed the source:\n--- want ---\n%s--- got ---\n%s", src, out)
	}
	if out2 := mustFormat(t, out); out2 != out {
		t.Fatalf("not idempotent:\n%s", out2)
	}
}

// Removing the nil check on Process's result (it is never nil, see
// preproc.TestProcessResultAndCleanAreNeverNil) changes no reachable behaviour.
// Measured on main before the removal, these are exactly what Format returned:
// an empty source formats to an empty output, an unparsable one is an error
// from the formatter, never a panic.
func TestFormatEdgeInputsAreUnchanged(t *testing.T) {
	for _, tc := range []struct {
		src     string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"package p\n", "package p\n", false},
		{"!", "", true},
		{"package p\n\nfunc f( {\n", "", true},
	} {
		out, err := Format("x.gon", []byte(tc.src))
		if (err != nil) != tc.wantErr || string(out) != tc.want {
			t.Fatalf("Format(%q) = %q, %v; want %q, error=%v", tc.src, out, err, tc.want, tc.wantErr)
		}
	}
}

// The tests below are the formatter controls of SF-2 (plan section 9): every
// expected output is written by hand, so the position of each ! does not come from
// running the preprocessor on the formatted text. A bug that the first and the
// second run share would pass a test that only compared the two runs.

func wrapConv(body string) string {
	return "package p\n\nfunc f() {\n" + body + "\n}\n"
}

func TestFormatNonNilConversionControls(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"one conversion", "\tv := (!*Node)(x)", "\tv := (!*Node)(x)"},
		{"two adjacent conversions", "\ta, b := (!*N)(x), (!*M)(y)", "\ta, b := (!*N)(x), (!*M)(y)"},
		{"two adjacent with spacing", "\ta,b:=(!*N)(x),(!*M)(y)", "\ta, b := (!*N)(x), (!*M)(y)"},
		{"nested conversion", "\tg((!*N)((!*M)(x)))", "\tg((!*N)((!*M)(x)))"},
		{"redundant two layers", "\tv := ((!*Node))(x)", "\tv := (!*Node)(x)"},
		{"redundant three layers", "\tv := (((!*Node)))(x)", "\tv := (!*Node)(x)"},
		{"redundant and spaced", "\tv := ( ( !*Node ) )(x)", "\tv := (!*Node)(x)"},
		{"redundant in an argument", "\tg(((!*N))(x))", "\tg((!*N)(x))"},
		{"redundant and not redundant", "\ta, b := ((!*N))(x), (!*M)(y)", "\ta, b := (!*N)(x), (!*M)(y)"},
		{"redundant twice", "\ta, b := ((!*N))(x), ((!*M))(y)", "\ta, b := (!*N)(x), (!*M)(y)"},
		{"qualified name", "\tv := ((!io.Reader))(x)", "\tv := (!io.Reader)(x)"},
		{"comment after the target", "\tv := ((!*N) /* c */ )(x)", "\tv := (!*N) /* c */ (x)"},
		{"comment inside the target", "\tv := (!*N /* c */)(x)", "\tv := (!*N /* c */)(x)"},
		{"comment before the call", "\tv := (!*N) /* c */ (x)", "\tv := (!*N) /* c */ (x)"},
		{"line comment after", "\tv := (!*N)(x) // c", "\tv := (!*N)(x) // c"},
		{"several bang in one target", "\tv := (!func(!*A) !*B)(x)", "\tv := (!func(!*A) !*B)(x)"},
		{"several bang in a map target", "\tv := (!map[string]!*T)(x)", "\tv := (!map[string]!*T)(x)"},
		{"run in the target", "\tv := (!!*A)(x)", "\tv := (!*A)(x)"},
		{"run and redundant", "\tv := ((!!!![]int))(x)", "\tv := (![]int)(x)"},
		{"after a block", "\tif ok {\n\t}\n\t(!*N)(x).Visit()", "\tif ok {\n\t}\n\t(!*N)(x).Visit()"},
		{"look-alike call", "\tw := adder(!flag)(3)", "\tw := adder(!flag)(3)"},
		{"expression double negation", "\tw := !!flag", "\tw := !!flag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := mustFormat(t, wrapConv(c.src))
			if want := wrapConv(c.want); out != want {
				t.Fatalf("got\n%s\nwant\n%s", out, want)
			}
			if again := mustFormat(t, out); again != out {
				t.Fatalf("not idempotent:\n%s\nthen\n%s", out, again)
			}
		})
	}
}

// An offset shift before a conversion: the removed ! and the removed
// parentheses move every later clean offset.
func TestFormatNonNilConversionAfterOffsetShift(t *testing.T) {
	src := "package p\n\n" +
		"var a   !!*int\n" +
		"var b !!!![]T\n\n" +
		"func f(p !!*N, q  !*M) {\n" +
		"\tv := ( ( !!*N ) )(p)\n" +
		"\tw := ((!*M))(q)\n" +
		"\t_, _ = v, w\n" +
		"}\n"
	want := "package p\n\n" +
		"var a !*int\n" +
		"var b ![]T\n\n" +
		"func f(p !*N, q !*M) {\n" +
		"\tv := (!*N)(p)\n" +
		"\tw := (!*M)(q)\n" +
		"\t_, _ = v, w\n" +
		"}\n"
	out := mustFormat(t, src)
	if out != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
	if mustFormat(t, out) != out {
		t.Fatalf("not idempotent:\n%s", out)
	}
}

func TestFormatBangRunInTypePosition(t *testing.T) {
	out := mustFormat(t, "package p\n\nvar x !!*int\nvar y []!!!*int\n")
	if want := "package p\n\nvar x !*int\nvar y []!*int\n"; out != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
}

// Formatting a source with conversions changes nothing but what go/format would
// change in the same source without the !.
func TestFormatNonNilConversionMakesNoUnrelatedChange(t *testing.T) {
	src := "package p\n\nfunc f( a  int ,b int)  int {\n  if a>b {return(a)}\n\tv:=((!*N))(x)\n\t_=v\n  return b\n}\n"
	out := mustFormat(t, src)
	plain := "package p\n\nfunc f( a  int ,b int)  int {\n  if a>b {return(a)}\n\tv:=((*N))(x)\n\t_=v\n  return b\n}\n"
	gofmted, err := Format("t.gon", []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(out, "!", "") != string(gofmted) {
		t.Fatalf("an unrelated change:\n%s\nvs\n%s", out, gofmted)
	}
	if !strings.Contains(out, "v := (!*N)(x)") {
		t.Fatalf("the conversion lost its !:\n%s", out)
	}
}

// Mismatch controls: a ! must never be dropped or placed wrongly. When the token
// streams cannot be aligned the formatter returns an error, so the caller leaves
// the file unformatted.
func TestFormatMismatchLeavesTheFileUnformatted(t *testing.T) {
	// Redundant parentheses around a whole conversion expression are outside the
	// tolerance, which covers only the parentheses of the target itself.
	src := wrapConv("\tv := (((!*N)(x)))")
	if out, err := Format("t.gon", []byte(src)); err == nil {
		t.Fatalf("expected an error, got\n%s", out)
	}
}

func TestReinsertBangsMismatches(t *testing.T) {
	cases := []struct {
		name      string
		clean     string
		nonNil    []int
		formatted string
	}{
		{"offset not at a token", "package p\nvar x *int\n", []int{15}, "package p\n\nvar x *int\n"},
		{"offset after the last token", "package p\nvar x *int\n", []int{100}, "package p\n\nvar x *int\n"},
		{"different token streams", "package p\nvar x *int\n", []int{16}, "package p\n\nvar y []int\n"},
		{"extra token in formatted", "package p\nvar x *int\n", []int{16}, "package p\n\nvar x *int = nil\n"},
		{"missing token in formatted", "package p\nvar x *int\n", []int{16}, "package p\n\nvar x\n"},
		{"parentheses removed without a conversion", "package p\nvar x = ((*int)(nil))\n", []int{20}, "package p\n\nvar x = (*int)(nil)\n"},
		{"parentheses removed from a parameter list", "package p\nfunc f((*int)) {}\n", []int{18}, "package p\n\nfunc f(*int) {}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nonNil := map[int]bool{}
			for _, o := range c.nonNil {
				nonNil[o] = true
			}
			if out, err := reinsertBangs([]byte(c.clean), nonNil, []byte(c.formatted)); err == nil {
				t.Fatalf("expected an error, got\n%s", out)
			}
		})
	}
}

// A marked offset that is not the start of a token is an error, never a silent
// drop of the contract, also when other marks in the file do match.
func TestFormatNeverDropsAMark(t *testing.T) {
	clean := "package p\nvar a *int\nvar b *int\n"
	nonNil := map[int]bool{16: true, 100: true}
	if out, err := reinsertBangs([]byte(clean), nonNil, []byte("package p\n\nvar a *int\nvar b *int\n")); err == nil {
		t.Fatalf("expected an error, got\n%s", out)
	}
}
