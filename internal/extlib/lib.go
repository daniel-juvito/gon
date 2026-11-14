// Package extlib is a tiny in-module library used to prove that
// external-package annotation resolution is generic (not stdlib-only).
package extlib

import "io"

// Handle is a stand-in receiver / pointer-field type for contract tests.
type Handle struct{}

// Take is a package-level function. The .gna contract may claim !*string
// (a `!` claim is only meaningful on a nilable kind, so s is a pointer).
func Take(s *string) error {
	return nil
}

// Put is a method. The .gna contract may claim ![]byte.
func (h *Handle) Put(b []byte) error {
	return nil
}

// Echo has no non-nil claim in the reference .gna (ordinary).
func Echo(s string) string {
	return s
}

// Open returns an interface value the reference .gna annotates "!io.Reader"
// (external interface result contract — M4b / E8).
func Open() io.Reader {
	return nil
}

// Conn carries external field contracts (see extlib.gna `types:`):
// DB and Log are "!*Handle", W is "!io.Writer".
type Conn struct {
	DB  *Handle
	Log *Handle
	W   io.Writer
}

// Base is an embedded type with its own "!" field contract (Root).
type Base struct {
	Root *Handle
}

// Wrapped embeds Base; a zero-value Wrapped leaves the promoted Base.Root
// contract unsatisfied (E6 one-hop embedded promotion).
type Wrapped struct {
	Base
	Name *Handle
}

// ---- generic symbols (v1.9 / M5b tests) ------------------------------------

// Mix has one non-nilable and one nilable parameter; tests use it to prove a
// bad `!` flag is dropped on its own position only.
func Mix(s string, p *int) error { return nil }

// Box is a generic struct whose field V carries a `.gna` contract ("!*X").
type Box[X any] struct{ V *X }

// Get returns the field; the reference .gna annotates the result "!*X".
func (b Box[X]) Get() *X { return b.V }

// Echo and Ptr declare their OWN type parameters (Go 1.27).
func (b Box[X]) Echo[Y any](y Y) Y { return y }
func (b Box[X]) Ptr[P ~*X](p P) P  { return p }

// Unwrap is generic over the element; params / results annotated "!*X".
func Unwrap[X any](p *X) *X { return p }

// Pick is generic over a slice-shaped type parameter (a nilable type set).
func Pick[S ~[]E, E any](s S) S { return s }

// Ident is generic over an unconstrained X; a `!X` claim on it is invalid.
func Ident[X any](x X) X { return x }

// Meta mixes a non-nilable field with a nilable one; tests flag both to prove
// an invalid field flag is dropped on its own field only.
type Meta struct {
	Label string
	H     *Handle
}
