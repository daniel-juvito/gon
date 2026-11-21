package preproc

import (
	"bytes"
	"strings"
	"testing"
)

func TestMultiReturnLocalSignatures(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// substrings that must appear in clean (with ! stripped)
		wantClean []string
		// substrings that must NOT appear (the ! forms)
		wantGone []string
	}{
		{
			name: "first result non-nil",
			src: `package main
func Open() (!*string, error) {
	s := "x"
	return &s, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!*string"},
		},
		{
			name: "second result non-nil",
			src: `package main
func Open() (*string, !error) {
	return nil, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!error"},
		},
		{
			name: "both results non-nil",
			src: `package main
func Open() (!*string, !error) {
	s := "x"
	return &s, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!*string", "!error"},
		},
		{
			name: "params and multi-return",
			src: `package main
func F(a !*int, b !*string) (!*int, error) {
	return a, nil
}
`,
			wantClean: []string{"func F(a *int, b *string) (*int, error)"},
			wantGone:  []string{"!*int", "!*string"},
		},
		{
			name: "unnamed params",
			src: `package main
func G(!*int, !*string) !*int {
	return nil
}
`,
			wantClean: []string{"func G(*int, *string) *int"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Process("t.gon", []byte(tc.src))
			clean := string(r.Clean)
			for _, w := range tc.wantClean {
				if !strings.Contains(clean, w) {
					t.Fatalf("clean missing %q:\n%s", w, clean)
				}
			}
			for _, g := range tc.wantGone {
				if strings.Contains(clean, g) {
					t.Fatalf("clean still contains %q:\n%s", g, clean)
				}
			}
			if len(r.NonNilOffsets) == 0 {
				t.Fatalf("expected some NonNilOffsets, clean:\n%s", clean)
			}
		})
	}
}

func TestElementPositionBangStripped(t *testing.T) {
	// v1.7 / M2b: element `!` is a type modifier after `]` (slice/array/map
	// value) and after `chan` / a channel direction arrow.
	cases := map[string]struct {
		src   string
		clean string
		offs  int
	}{
		"slice":    {"package p\nvar x []!*T\n", "package p\nvar x []*T\n", 1},
		"array":    {"package p\nvar x [3]!*T\n", "package p\nvar x [3]*T\n", 1},
		"ellipsis": {"package p\nvar x = [...]!*T{}\n", "package p\nvar x = [...]*T{}\n", 1},
		"mapval":   {"package p\nvar x map[string]!*T\n", "package p\nvar x map[string]*T\n", 1},
		"mapkey":   {"package p\nvar x map[!*K]V\n", "package p\nvar x map[*K]V\n", 1},
		"chan":     {"package p\nvar x chan !*T\n", "package p\nvar x chan *T\n", 1},
		"recvchan": {"package p\nvar x <-chan !*T\n", "package p\nvar x <-chan *T\n", 1},
		"sendchan": {"package p\nvar x chan<- !*T\n", "package p\nvar x chan<- *T\n", 1},
		"outer":    {"package p\nvar x ![]!*T\n", "package p\nvar x []*T\n", 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := Process("t.gon", []byte(tc.src))
			if string(r.Clean) != tc.clean {
				t.Fatalf("clean mismatch:\n got %q\nwant %q", string(r.Clean), tc.clean)
			}
			if len(r.NonNilOffsets) != tc.offs {
				t.Fatalf("want %d offsets, got %d", tc.offs, len(r.NonNilOffsets))
			}
		})
	}
}

func TestElementPositionBangDoesNotTouchExpressions(t *testing.T) {
	// `ch <- !flag` (send) and `m[!ok]` (index) are ordinary unary NOT.
	src := `package main
func f(ch chan bool, m map[bool]int, flag bool, ok bool) {
	ch <- !flag
	_ = m[!ok]
}
`
	r := Process("t.gon", []byte(src))
	if len(r.NonNilOffsets) != 0 {
		t.Fatalf("unary ! in expressions must not be recorded: %v", r.NonNilOffsets)
	}
	if !strings.Contains(string(r.Clean), "!flag") || !strings.Contains(string(r.Clean), "m[!ok]") {
		t.Fatalf("unary ! was stripped:\n%s", string(r.Clean))
	}
}

func TestUnaryNotNotStripped(t *testing.T) {
	src := `package main
func f(a bool, b bool) bool {
	return f(a, !b) || (!a) || !b
}
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	// Unary ! must remain.
	if !strings.Contains(clean, "!b") {
		t.Fatalf("unary !b was stripped:\n%s", clean)
	}
	if !strings.Contains(clean, "(!a)") {
		t.Fatalf("unary (!a) was stripped:\n%s", clean)
	}
}

func TestTypeAssertBangStripped(t *testing.T) {
	// x.(!T) is not a Gon mechanism, but the ! is a type modifier there:
	// strip it and record the offset so the checker reports it cleanly
	// instead of the parser choking (RFC interface-semantics D6a).
	src := `package main
func f(x any) {
	r := x.(!T)
	v, ok := x.(!T)
	_, _, _ = r, v, ok
}
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	if strings.Contains(clean, "!T") {
		t.Fatalf("! not stripped from type-assertion target:\n%s", clean)
	}
	if strings.Count(clean, "x.(T)") != 2 {
		t.Fatalf("expected two stripped assertions:\n%s", clean)
	}
	if len(r.NonNilOffsets) != 2 {
		t.Fatalf("expected 2 recorded non-nil offsets, got %d", len(r.NonNilOffsets))
	}
}

func TestUnaryNotAfterParenNotStripped(t *testing.T) {
	// A LPAREN that is not `.(` must not trigger type-assertion stripping.
	src := `package main
func f(a bool) bool {
	return (!a)
}
`
	r := Process("t.gon", []byte(src))
	if !strings.Contains(string(r.Clean), "(!a)") {
		t.Fatalf("unary ! after plain ( was stripped:\n%s", string(r.Clean))
	}
}

func TestClassicStillWorks(t *testing.T) {
	src := `package main
type S struct { X !*int }
func f(x !*int) !*int { return x }
func (r !*S) M() {}
var y !*int
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	if strings.Contains(clean, "!") {
		// only unary would remain; there should be none here
		t.Fatalf("unexpected ! left in clean:\n%s", clean)
	}
	if !bytes.Contains(r.Clean, []byte("func f(x *int) *int")) {
		t.Fatalf("classic signature broken:\n%s", clean)
	}
}

func TestInterfaceMethodMultiReturn(t *testing.T) {
	src := `package main
type I interface {
	Open() (!*string, error)
}
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	if !strings.Contains(clean, "Open() (*string, error)") {
		t.Fatalf("interface method multi-return not stripped:\n%s", clean)
	}
	if strings.Contains(clean, "!*string") {
		t.Fatalf("! left in interface method:\n%s", clean)
	}
}

// v1.8 / M3: `!` in type-argument and constraint positions is stripped so the
// checker can report GN003 (RFC G4 / G9) instead of a Go parse error, while
// ordinary unary `!` inside index expressions is untouched.
func TestGenericTypeArgumentAndConstraintBang(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantClean string
		wantBangs int
	}{
		{"first type arg", "var d Box[!*T]\n", "var d Box[*T]", 1},
		{"second type arg", "var d Reg[string, !*T]\n", "var d Reg[string, *T]", 1},
		{"call type arg", "func g() { F[!*T]() }\n", "F[*T]()", 1},
		{"constraint tilde", "func F[P ~!*T](p P) {}\n", "func F[P ~*T](p P) {}", 1},
		{"slice type arg", "var d Box[![]T]\n", "var d Box[[]T]", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Process("t.gon", []byte("package main\n"+tc.src))
			if !strings.Contains(string(r.Clean), tc.wantClean) {
				t.Fatalf("clean = %q, want it to contain %q", r.Clean, tc.wantClean)
			}
			if len(r.NonNilOffsets) != tc.wantBangs {
				t.Fatalf("got %d offsets, want %d", len(r.NonNilOffsets), tc.wantBangs)
			}
		})
	}
}

func TestIndexUnaryNotUntouched(t *testing.T) {
	src := "package main\nfunc f(m map[bool]int, ok bool, p *bool) {\n\t_ = m[!ok]\n\t_ = m[!(ok)]\n\t_ = m[!f2()]\n}\n"
	r := Process("t.gon", []byte(src))
	if string(r.Clean) != src {
		t.Fatalf("unary ! in index expression must not be stripped:\n%s", r.Clean)
	}
	if len(r.NonNilOffsets) != 0 {
		t.Fatalf("expected no offsets, got %v", r.NonNilOffsets)
	}
}

// Process never returns nil, and its Clean is never nil, for any input: the
// `!*Result` contract (dogfood D5) and the reason fmt.Format needs no nil check.
// Clean is built with make([]byte, 0, len(src)), so even an empty source gives
// a non-nil, empty slice.
func TestProcessResultAndCleanAreNeverNil(t *testing.T) {
	for _, src := range []string{
		"",
		"   \n\t",
		"!",
		"!!!",
		"\x00\xff not Go at all",
		"package p\n",
		"package p\n\nvar x !*int\n",
		"package p\n\nfunc f( {\n",
	} {
		r := Process("x.gon", []byte(src))
		if r == nil {
			t.Fatalf("Process(%q) returned nil", src)
		}
		if r.Clean == nil {
			t.Fatalf("Process(%q).Clean is nil", src)
		}
	}
}

// markOffsets renders Clean with a "<!>" before every recorded offset.
func markOffsets(r *Result) string {
	var b strings.Builder
	for i := 0; i <= len(r.Clean); i++ {
		if r.NonNilOffsets[i] {
			b.WriteString("<!>")
		}
		if i < len(r.Clean) {
			b.WriteByte(r.Clean[i])
		}
	}
	return b.String()
}

// TestNonNilConversionRecognised covers RFC section 6.1: the expected marked
// source is written by hand next to each input.
func TestNonNilConversionRecognised(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"pointer", "v := (!*N)(x)", "v := (<!>*N)(x)"},
		{"slice", "v := (![]T)(x)", "v := (<!>[]T)(x)"},
		{"qualified", "v := (!io.Reader)(x)", "v := (<!>io.Reader)(x)"},
		{"nested two", "v := ((!*N))(x)", "v := ((<!>*N))(x)"},
		{"nested three spaced", "v := ( ( ( !*N ) ) )(x)", "v := ( ( ( <!>*N ) ) )(x)"},
		{"comment before the bang", "v := (/* c */ !*N)(x)", "v := (/* c */ <!>*N)(x)"},
		{"comment before the call", "v := (!*N) /* c */ (x)", "v := (<!>*N) /* c */ (x)"},
		{"argument", "g((!*N)(x))", "g((<!>*N)(x))"},
		{"after a keyword", "return (!*N)(x)", "return (<!>*N)(x)"},
		{"two in one statement", "a, b := (!*N)(x), (!*M)(y)", "a, b := (<!>*N)(x), (<!>*M)(y)"},
		{"index operand", "v := a[(!*N)(x).i]", "v := a[(<!>*N)(x).i]"},
		{"after a semicolon", "a; (!*N)(x).Visit()", "a; (<!>*N)(x).Visit()"},
		{"after a block", "if ok {\n}\n(!*N)(x).Visit()", "if ok {\n}\n(<!>*N)(x).Visit()"},
		{"run in the target", "v := (!!*N)(x)", "v := (<!>*N)(x)"},
		{"long run in the target", "v := (!!!!![]int)(x)", "v := (<!>[]int)(x)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pre := "package p\n\nfunc f() {\n"
			r := Process("t.gon", []byte(pre+c.src+"\n}\n"))
			if got, want := markOffsets(r), pre+c.want+"\n}\n"; got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			checkCleanToOrig(t, []byte(pre+c.src+"\n}\n"), r)
		})
	}
}

// checkCleanToOrig requires CleanToOrig to map every byte of Clean to the same
// byte of the original source, in increasing order (Protocol v1 positions depend
// on it).
func checkCleanToOrig(t *testing.T, src []byte, r *Result) {
	t.Helper()
	if len(r.CleanToOrig) != len(r.Clean) {
		t.Fatalf("CleanToOrig has %d entries for %d clean bytes", len(r.CleanToOrig), len(r.Clean))
	}
	prev := -1
	for i, o := range r.CleanToOrig {
		if o <= prev || o >= len(src) || src[o] != r.Clean[i] {
			t.Fatalf("CleanToOrig[%d] = %d does not map %q to the same byte of the source", i, o, r.Clean[i])
		}
		prev = o
	}
}

// TestNonNilConversionLookAlikesUnchanged is the "looks similar but keeps its old
// meaning" criterion: each source keeps its clean source and has no offset.
func TestNonNilConversionLookAlikesUnchanged(t *testing.T) {
	cases := []struct{ name, src string }{
		{"call result called", "w := adder(!flag)(3)"},
		{"selector call result called", "w := p.f(!a)(b)"},
		{"call of a call", "w := f(x)(!a)(b)"},
		{"index then call", "w := m[k](!a)(b)"},
		{"function literal called", "w := func() {}(!a)(b)"},
		{"call paren then wrapper", "w := f((!a))(b)"},
		{"literals", "w := 1(!a)(b)"},
		{"string literal", "w := \"s\"(!a)(b)"},
		{"comment between", "w := adder /* c */ (!flag)(3)"},
		{"negation then operator", "ok := (!a) && b"},
		{"negation alone", "ok := (!a)"},
		{"negation then call of a call", "ok := (!a)\nreturn"},
		{"double negation", "ok := !!a"},
		{"double negation in parentheses", "ok := (!!a) && b"},
		{"negated parenthesis", "ok := !(!a)"},
		{"unbalanced", "v := (!*N)(x"},
		{"no call after", "v := (!*N)"},
		{"close then operator", "v := (!*N) + 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n" + c.src + "\n}\n"
			r := Process("t.gon", []byte(src))
			if string(r.Clean) != src || len(r.NonNilOffsets) != 0 {
				t.Errorf("changed:\nclean   %q\noffsets %v", r.Clean, r.NonNilOffsets)
			}
		})
	}
}

// A source that does not scan cleanly is left alone (RFC section 6.1, the
// conservative rule), while the type positions that exist today still work.
func TestNonNilConversionNeedsACleanScan(t *testing.T) {
	src := "package p\n\nvar bad = 'ab'\n\nfunc f() {\n\tv := (!*N)(x)\n}\n"
	r := Process("t.gon", []byte(src))
	if string(r.Clean) != src || len(r.NonNilOffsets) != 0 {
		t.Errorf("a conversion was recognised in a source that does not scan:\n%q\n%v", r.Clean, r.NonNilOffsets)
	}
	src = "package p\n\nvar bad = 'ab'\n\nvar x !*int\n"
	r = Process("t.gon", []byte(src))
	if strings.Contains(string(r.Clean), "!") || len(r.NonNilOffsets) != 1 {
		t.Errorf("an ordinary type position stopped working in a source with a scan error:\n%q\n%v", r.Clean, r.NonNilOffsets)
	}
}

// RFC section 6.7: a run of ! in a type position is one !, in every type position
// that exists, and an expression position is never touched.
func TestRunOfBangInTypePosition(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"var", "var x !!*int", "var x <!>*int"},
		{"result", "func f() !!!*int { return nil }", "func f() <!>*int { return nil }"},
		{"parameter", "func f(a !!*int, b !![]T) {}", "func f(a <!>*int, b <!>[]T) {}"},
		{"multi result", "func f() (!!*int, error) { return nil, nil }", "func f() (<!>*int, error) { return nil, nil }"},
		{"element", "var x []!!*int", "var x []<!>*int"},
		{"map value", "var x map[string]!!*int", "var x map[string]<!>*int"},
		{"channel", "var x chan !!*int", "var x chan <!>*int"},
		{"field", "type S struct{ f !!*int }", "type S struct{ f <!>*int }"},
		{"expression untouched", "var ok = !!f", "var ok = !!f"},
		{"expression in a call", "var v = g(a, !!f)", "var v = g(a, !!f)"},
		{"expression after a modifier line", "var x !*int; var ok = !!f", "var x <!>*int; var ok = !!f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package p\n\n" + c.src + "\n"
			r := Process("t.gon", []byte(src))
			if got, want := markOffsets(r), "package p\n\n"+c.want+"\n"; got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			checkCleanToOrig(t, []byte(src), r)
		})
	}
}

// Running the preprocessor on its own clean output must find nothing more.
func TestProcessCleanIsStable(t *testing.T) {
	src := "package p\n\nvar x !!*int\n\nfunc f() {\n\tv := ((!!*N))(x)\n}\n"
	r := Process("t.gon", []byte(src))
	again := Process("t.gon", r.Clean)
	if string(again.Clean) != string(r.Clean) || len(again.NonNilOffsets) != 0 {
		t.Errorf("the clean source is not stable:\n%q\n%v", again.Clean, again.NonNilOffsets)
	}
}
