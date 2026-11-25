package checker

import (
	"strings"
	"testing"
)

// Rules A, B and C1 of the RFC on local backdoors (accepted 2026-11-25): a
// zero-valued array of `!` elements and a `make` of a slice of `!` elements are
// GN002, and a named `!T` result is a `!` variable in the body of a declaration.

const zvPreamble = `package main

type T struct{}
type Slots [4]!*T
type Ptrs []!*T
type Holder struct{ A [2]!*T }

func ord() *T { return nil }
func nn() !*T { return &T{} }

const zero = 0

`

func zvCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, zvPreamble+body)
}

func zvWant(t *testing.T, d []*Diagnostic, code string, n int) {
	t.Helper()
	if got := countCode(d, code); got != n {
		t.Fatalf("expected %d %s, got %d: %v", n, code, got, d)
	}
}

func TestZV_A_ZeroValuedArraySites(t *testing.T) {
	for name, body := range map[string]string{
		"var":         `func f() { var a [3]!*T; _ = a }`,
		"var package": `var g [3]!*T`,
		"new":         `func f() { _ = new([3]!*T) }`,
		"named type":  `func f() { var s Slots; _ = s }`,
		"named new":   `func f() { _ = new(Slots) }`,
		"struct var":  `func f() { var h Holder; _ = h }`,
		"struct lit":  `func f() { _ = Holder{} }`,
		"struct new":  `func f() { _ = new(Holder) }`,
		"map of":      `func f() { var a [2]![]T; _ = a }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := zvCheck(t, body)
			zvWant(t, d, "GN002", 1)
			if !strings.Contains(d[0].Message, "zero-valued array") {
				t.Errorf("message = %q", d[0].Message)
			}
		})
	}
}

func TestZV_A_SilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"zero length":      `func f() { var a [0]!*T; _ = a }`,
		"full literal":     `func f() { _ = [2]!*T{nn(), nn()} }`,
		"initialiser":      `func f() { var a [2]!*T = [2]!*T{nn(), nn()}; _ = a }`,
		"plain pointers":   `func f() { var a [3]*T; _ = a }`,
		"no element bang":  `func f() { var a [3]int; _ = a }`,
		"slice":            `func f() { var s []!*T; _ = s }`,
		"inferred length":  `func f() { _ = [...]!*T{nn(), nn()} }`,
		"pointer to array": `func f() { var p *[3]!*T; _ = p }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 0)
		})
	}
}

func TestZV_A_StructFieldNamesTheField(t *testing.T) {
	d := zvCheck(t, `func f() { var h Holder; _ = h }`)
	zvWant(t, d, "GN002", 1)
	if !strings.Contains(d[0].Message, "field A") {
		t.Errorf("message = %q, want the field named", d[0].Message)
	}
}

func TestZV_B_MakeSlice(t *testing.T) {
	for name, body := range map[string]string{
		"len 3":         `func f() { _ = make([]!*T, 3) }`,
		"variable":      `func f(n int) { _ = make([]!*T, n) }`,
		"with cap":      `func f() { _ = make([]!*T, 2, 8) }`,
		"named slice":   `func f(n int) { _ = make(Ptrs, n) }`,
		"const nonzero": `func f() { const k = 2; _ = make([]!*T, k) }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 1)
		})
	}
}

func TestZV_B_SilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"len 0":         `func f() { _ = make([]!*T, 0) }`,
		"len 0 cap n":   `func f(n int) { _ = make([]!*T, 0, n) }`,
		"const zero":    `func f() { _ = make([]!*T, zero) }`,
		"plain slice":   `func f(n int) { _ = make([]*T, n) }`,
		"map":           `func f(n int) { _ = make(map[string]!*T, n) }`,
		"channel":       `func f(n int) { _ = make(chan *T, n) }`,
		"user function": `func make(a []int, n int) {}; func g() { make(nil, 3) }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 0)
		})
	}
}

// The known limit that rule B does not close: a capacity makes elements that a
// reslice exposes. The rule looks at the length only.
func TestZV_B_ResliceOverCapacityStaysAccepted(t *testing.T) {
	zvWant(t, zvCheck(t, `func f() { s := make([]!*T, 0, 3); s = s[:3]; _ = s }`), "GN002", 0)
}

func TestZV_C1_NamedResultIsABangVariable(t *testing.T) {
	d := zvCheck(t, `func f() (x !*T) { x = nil; return nn() }`)
	zvWant(t, d, "GN001", 1)
	d = zvCheck(t, `func f() (x !*T) { x = ord(); return nn() }`)
	zvWant(t, d, "GN001", 1)
	d = zvCheck(t, `func f() (x !*T) { x = nn(); if x == nil {}; return nn() }`)
	zvWant(t, d, "GW001", 1)
	zvWant(t, d, "GN001", 0)
}

func TestZV_C1_NamedResultSilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"plain result":    `func f() (x *T) { x = nil; return x }`,
		"mixed":           `func f() (x !*T, y *T) { x = nn(); y = nil; return x, y }`,
		"source assigned": `func f() (x !*T) { x = &T{}; return x }`,
		"unnamed":         `func f() !*T { return nn() }`,
		"method":          `type R struct{}; func (R) m() (x *T) { x = nil; return }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := zvCheck(t, body)
			zvWant(t, d, "GN001", 0)
			zvWant(t, d, "GW001", 0)
		})
	}
}

func TestZV_C1_MethodNamedBangResult(t *testing.T) {
	d := zvCheck(t, `type R struct{}; func (R) m() (x !*T) { x = nil; return nn() }`)
	zvWant(t, d, "GN001", 1)
}

// Chains of defined types and aliases are followed to the array or slice at the end
// of them, however long they are.
func TestZV_A_NamedArrayChains(t *testing.T) {
	const decls = `type A [4]!*T
type B A
type C B
type D = C
type Holder2 struct{ S D }
`
	for name, body := range map[string]string{
		"defined once":   `func f() { var b B; _ = b }`,
		"defined twice":  `func f() { var c C; _ = c }`,
		"alias":          `func f() { var d D; _ = d }`,
		"new":            `func f() { _ = new(C) }`,
		"new of alias":   `func f() { _ = new(D) }`,
		"struct field":   `func f() { var h Holder2; _ = h }`,
		"struct literal": `func f() { _ = Holder2{} }`,
		"struct new":     `func f() { _ = new(Holder2) }`,
		"package level":  `var g C`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, checkTyped(t, "package main\n\ntype T struct{}\n"+decls+body), "GN002", 1)
		})
	}
}

func TestZV_B_LongSliceChain(t *testing.T) {
	var b strings.Builder
	b.WriteString("package main\n\ntype T struct{}\ntype S0 []!*T\n")
	for i := 1; i <= 30; i++ {
		b.WriteString("type S" + itoa(i) + " S" + itoa(i-1) + "\n")
	}
	b.WriteString("func f(n int) { _ = make(S30, n); _ = make(S30, 0, n) }\n")
	zvWant(t, checkTyped(t, b.String()), "GN002", 1)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for ; i > 0; i /= 10 {
		d = append([]byte{byte('0' + i%10)}, d...)
	}
	return string(d)
}

// Without type information a function called make cannot be told from the builtin,
// so rule B is silent instead of risking a wrong GN002.
func TestZV_B_NoTypeInformationIsSilent(t *testing.T) {
	d := checkSource(t, `package main

type T struct{}

func make(n int) []int { return nil }

func f() { _ = make(3); _ = make([]!*T, 3) }
`)
	zvWant(t, d, "GN002", 0)
}

func TestZV_A_KeyedFullLiteralIsSilent(t *testing.T) {
	zvWant(t, zvCheck(t, `func f() { _ = [3]!*T{2: nn(), 0: nn(), 1: nn()} }`), "GN002", 0)
}

func TestZV_C1_MultipleNamedResultsAndShadowing(t *testing.T) {
	d := zvCheck(t, `func f() (a, b !*T) { a = nil; b = ord(); return nn(), nn() }`)
	zvWant(t, d, "GN001", 2)
	// An inner variable of the same name is its own variable.
	d = zvCheck(t, `func f() (x !*T) { { x := ord(); x = nil; _ = x }; return nn() }`)
	zvWant(t, d, "GN001", 0)
}

// A type is the declaration that Go's own scope resolution picks: a local type that
// shadows a package type, a type declared inside a function, and a type declared in a
// block that shadows another of the same name.
func TestZV_Scope_LocalTypesAndShadowing(t *testing.T) {
	const decls = `type Slots [4]!*T
type Ptrs []!*T
`
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"local int shadows a package array":   {`func f() { type Slots int; var a Slots; _ = a }`, 0},
		"local []int shadows a package slice": {`func f(n int) { type Ptrs []int; _ = make(Ptrs, n) }`, 0},
		"local named array":                   {`func f() { type Local [4]!*T; var a Local; _ = a }`, 1},
		"local alias of a local array":        {`func f() { type Local [4]!*T; type Alias = Local; var a Alias; _ = a }`, 1},
		"local alias of a package array":      {`func f() { type Alias = Slots; var a Alias; _ = a }`, 1},
		"local named slice in make":           {`func f(n int) { type L []!*T; _ = make(L, n) }`, 1},
		"package array without shadowing":     {`func f() { var a Slots; _ = a }`, 1},
		"inner block shadows an outer local":  {`func f() { type X [2]!*T; { type X int; var a X; _ = a } }`, 0},
		"inner block array, outer is not":     {`func f() { type X int; { type X [2]!*T; var a X; _ = a } }`, 1},
		"other function is not in scope":      {`func g() { type Slots int }; func f() { var a Slots; _ = a }`, 1},
		"a type parameter is not an array":    {`func f[P any]() { var p P; _ = p }`, 0},
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, checkTyped(t, "package main\n\ntype T struct{}\n"+decls+c.body), "GN002", c.want)
		})
	}
}

// Without type information a name cannot be told from a shadowing one, so a named type
// is not resolved; an array written out in the declaration still is.
func TestZV_Scope_NoTypeInformation(t *testing.T) {
	d := checkSource(t, `package main

type T struct{}
type Slots [4]!*T

func f() { var named Slots; var literal [3]!*T; _, _ = named, literal }
`)
	zvWant(t, d, "GN002", 1)
}
