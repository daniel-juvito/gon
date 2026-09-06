// Type Coverage (v1.6 / M1b).
//
// Executable specification for docs/rfc-type-coverage.md §8. `!` on a nilable
// reference kind (slice, map, chan, func, pointer) means the reference value
// is non-nil — nothing about length, contents, or channel state.
//
// Observable discriminants:
//   GN001 — literal nil into a ! reference position; ! type-assertion target
//   GN002 — bare `var x !S` (nilable ref kind) with no initializer
//   GN003 — ! on a non-nilable kind (struct, array, basic)
//   GW001 — nil comparison against a known ! reference source
package checker

import "testing"

// --- C3: nil literal into a ! reference position -----------------------------

func TestTC_NilIntoBangSliceVar(t *testing.T) {
	diags := checkTyped(t, `package main
func f() {
	var s ![]byte = nil
	_ = s
}`)
	if countCode(diags, "GN001") != 1 {
		t.Fatalf("nil into ![]byte var must be GN001: %v", diags)
	}
}

func TestTC_NilIntoBangMapParamAndReturn(t *testing.T) {
	diags := checkTyped(t, `package main
func take(m !map[string]int) {}
func give() !chan struct{} { return nil }
func f() { take(nil) }`)
	if countCode(diags, "GN001") != 2 {
		t.Fatalf("nil arg to !map param and nil return of !chan must be 2×GN001: %v", diags)
	}
}

func TestTC_NilIntoBangFuncFieldLiteral(t *testing.T) {
	diags := checkTyped(t, `package main
type Mux struct{ h !func() }
func f() { _ = Mux{h: nil} }`)
	if countCode(diags, "GN001") != 1 {
		t.Fatalf("explicit nil for !func field must be GN001: %v", diags)
	}
}

// --- C5: bare `var x !S` with no initializer ---------------------------------

func TestTC_ZeroValueBareVar_AllRefKinds(t *testing.T) {
	cases := map[string]string{
		"slice":   `var s ![]byte`,
		"map":     `var m !map[string]int`,
		"chan":    `var c !chan int`,
		"chandir": `var c !<-chan int`,
		"func":    `var fn !func() error`,
		"pointer": `var p !*int`,
	}
	for name, decl := range cases {
		t.Run(name, func(t *testing.T) {
			diags := checkTyped(t, "package main\nfunc f() {\n\t"+decl+"\n\t_ = "+firstName(decl)+"\n}")
			if countCode(diags, "GN002") != 1 {
				t.Fatalf("%s: bare `var x !S` must be GN002: %v", name, diags)
			}
		})
	}
}

func TestTC_ZeroValueBareVar_PackageScope(t *testing.T) {
	diags := checkTyped(t, `package main
var Routes !map[string]int
func f() { _ = Routes }`)
	if countCode(diags, "GN002") != 1 {
		t.Fatalf("package-scope bare `var !map` must be GN002: %v", diags)
	}
}

func TestTC_ZeroValueBareVar_NamedAndAlias(t *testing.T) {
	diags := checkTyped(t, `package main
type Handler func()
type Bytes = []byte
func f() {
	var h !Handler
	var b !Bytes
	_, _ = h, b
}`)
	if countCode(diags, "GN002") != 2 {
		t.Fatalf("named func type + slice alias must each be GN002: %v", diags)
	}
}

func TestTC_ZeroValueVar_WithInitializerAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
func f() {
	var s ![]byte = make([]byte, 0)
	var m !map[string]int = map[string]int{}
	var fn !func() = func() {}
	_, _, _ = s, m, fn
}`)
	if len(diags) != 0 {
		t.Fatalf("! reference vars with non-nil initializers must be silent: %v", diags)
	}
}

func TestTC_ZeroValueVar_InterfaceExcluded(t *testing.T) {
	diags := checkTyped(t, `package main
type R interface{ M() }
func f() {
	var r !R
	_ = r
}`)
	if countCode(diags, "GN002") != 0 {
		t.Fatalf("bare `var r !I` is owned by M1a and must NOT be GN002: %v", diags)
	}
}

// --- C2: non-nil is not non-empty ------------------------------------------

func TestTC_EmptyCompositeAndRangeAccepted(t *testing.T) {
	diags := checkTyped(t, `package main
func handlers() ![]int { return []int{} }
func f() {
	s := handlers()
	_ = len(s)
	for range s {}
}`)
	if len(diags) != 0 {
		t.Fatalf("empty ! slice, len(), and range must be silent: %v", diags)
	}
}

// --- C6: no propagation through derived expressions -------------------------

func TestTC_AppendResultIsOrdinary(t *testing.T) {
	diags := checkTyped(t, `package main
func base() ![]int { return []int{} }
func f() {
	s := base()
	x := append(s, 1)
	if x == nil {}
}`)
	if countCode(diags, "GW001") != 0 {
		t.Fatalf("append result must not be a ! source (no GW001): %v", diags)
	}
}

func TestTC_ResliceAndConversionOrdinary(t *testing.T) {
	diags := checkTyped(t, `package main
type Bytes []byte
func base() ![]byte { return []byte{} }
func f() {
	s := base()
	a := s[1:]
	b := Bytes(s)
	if a == nil {}
	if b == nil {}
}`)
	if countCode(diags, "GW001") != 0 {
		t.Fatalf("reslice / conversion must not propagate ! (no GW001): %v", diags)
	}
}

// --- C7: ! type-assertion target ------------------------------------------

func TestTC_BangAssertionTarget(t *testing.T) {
	diags := checkTyped(t, `package main
func f(x any) {
	_ = x.(![]byte)
	_ = x.(!*int)
}`)
	if countCode(diags, "GN001") != 2 {
		t.Fatalf("! on any assertion target must be GN001: %v", diags)
	}
}

// --- O4: ! on a non-nilable kind -----------------------------------------

func TestTC_BangOnNonNilableKind(t *testing.T) {
	diags := checkTyped(t, `package main
type Point struct{ X, Y int }
func f() {
	var p !Point
	var n !int
	var a ![3]int
	_, _, _ = p, n, a
}`)
	if countCode(diags, "GN003") != 3 {
		t.Fatalf("! on struct / basic / array must each be GN003: %v", diags)
	}
	if countCode(diags, "GN002") != 0 {
		t.Fatalf("non-nilable ! must not also emit GN002: %v", diags)
	}
}

// --- C11: ! reference-kind struct field ----------------------------------

func TestTC_BangSliceField_Construction(t *testing.T) {
	diags := checkTyped(t, `package main
type Reg struct{ names !map[string]int }
func f() { _ = Reg{} }`)
	if countCode(diags, "GN002") != 1 {
		t.Fatalf("zero construction of a !map field must be GN002: %v", diags)
	}
}

func TestTC_BangSliceField_MutationAndSource(t *testing.T) {
	diags := checkTyped(t, `package main
type Reg struct{ items ![]int }
func f() {
	r := Reg{items: []int{}}
	r.items = nil
	if r.items == nil {}
}`)
	if countCode(diags, "GN001") != 1 {
		t.Fatalf("r.items = nil must be GN001: %v", diags)
	}
	if countCode(diags, "GW001") != 1 {
		t.Fatalf("selector of a ! field is a non-nil source → GW001: %v", diags)
	}
}

func TestTC_ContainmentWalkUnchanged(t *testing.T) {
	// A slice/map/chan/func field whose ELEMENT type has a ! field must not
	// be traversed — the zero-value containment boundary is unchanged.
	diags := checkTyped(t, `package main
type Inner struct{ c !*int }
type Outer struct{
	s []Inner
	m map[string]Inner
}
func f() { _ = Outer{} }`)
	if countCode(diags, "GN002") != 0 {
		t.Fatalf("containment walk must stop at slice/map boundaries: %v", diags)
	}
}

// --- C8: ! result of a reference kind is a non-nil source ----------------

func TestTC_BangSliceResultIsSource(t *testing.T) {
	diags := checkTyped(t, `package main
func load() ![]int { return []int{} }
func f() {
	s := load()
	if s == nil {}
}`)
	if countCode(diags, "GW001") != 1 {
		t.Fatalf("annotated ![]int result must be a non-nil source → GW001: %v", diags)
	}
}

// firstName extracts the identifier from a `var NAME TYPE` declaration string.
func firstName(decl string) string {
	fields := splitFields(decl)
	if len(fields) >= 2 {
		return fields[1]
	}
	return "_"
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
