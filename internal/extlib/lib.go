// Package extlib is a tiny in-module library used to prove that
// external-package annotation resolution is generic (not stdlib-only).
package extlib

import "io"

// Handle is a stand-in receiver / pointer-field type for contract tests.
type Handle struct{}

// Take is a package-level function. The .gna contract may claim !string.
func Take(s string) error {
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
