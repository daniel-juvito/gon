// Methods and plain functions must not share contract tables.
//
// funcParams / funcReturns used to be keyed by the BARE declaration name, for
// plain functions and methods alike, and funcReturns also drove the `return`
// check inside each body. A method named like a function (or like a method of
// another type) therefore overwrote its contract: the declaration seen last
// won. Observable as false GN001 / GW001 on the plain function, or a missed
// GN001 inside a method body.
package checker

import "testing"

const collPre = `package main
type T struct{}
type Box struct{}
type A struct{}
type B struct{}
`

func collCheck(t *testing.T, body string) []*Diagnostic {
	t.Helper()
	return checkTyped(t, collPre+body)
}

// A plain function and a LATER method share a name; only the method has a `!`.
func TestMethodDoesNotLeakIntoSameNamedFunction(t *testing.T) {
	d := collCheck(t, `
func Put(p *T) {}
func (b *Box) Put(p !*T) {}
func Make() *T { return nil }
func (b *Box) Make() !*T { return &T{} }
func main() {
	Put(nil)
	m := Make()
	if m == nil {}
}`)
	if len(d) != 0 {
		t.Fatalf("a plain function must be unaffected by a same-named method: %v", d)
	}
}

// Same, with the method declared FIRST.
func TestMethodDoesNotLeakIntoSameNamedFunctionMethodFirst(t *testing.T) {
	d := collCheck(t, `
func (b *Box) Make() !*T { return &T{} }
func Make() *T { return nil }
func main() { m := Make(); if m == nil {} }`)
	if len(d) != 0 {
		t.Fatalf("a plain function must be unaffected by a same-named method: %v", d)
	}
}

// Two types share a method name; only A's result carries the contract. B's
// body may return nil and A's may not, whatever the declaration order.
func TestMethodsOfDifferentTypesDoNotShareBodyContracts(t *testing.T) {
	for name, src := range map[string]string{
		"A first": `
func (a *A) Get() !*T { return nil }
func (b *B) Get() *T { return nil }`,
		"B first": `
func (b *B) Get() *T { return nil }
func (a *A) Get() !*T { return nil }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := collCheck(t, src)
			if got := countCode(d, "GN001"); got != 1 {
				t.Fatalf("exactly A.Get's `return nil` must be GN001, got %d: %v", got, d)
			}
		})
	}
}

// The plain function's own body contract is still enforced.
func TestPlainFunctionBodyContractStillEnforced(t *testing.T) {
	d := collCheck(t, `
func Make() !*T { return nil }
func (b *Box) Make() *T { return nil }`)
	if got := countCode(d, "GN001"); got != 1 {
		t.Fatalf("plain Make's `return nil` must be GN001, got %d: %v", got, d)
	}
}

// Plain-function call contracts keep working next to a same-named method.
func TestPlainFunctionCallContractUnaffected(t *testing.T) {
	d := collCheck(t, `
func Put(p !*T) {}
func (b *Box) Put(p *T) {}
func main() { Put(nil) }`)
	if got := countCode(d, "GN001"); got != 1 {
		t.Fatalf("Put(nil) must be GN001 (the function's contract), got %d: %v", got, d)
	}
}
