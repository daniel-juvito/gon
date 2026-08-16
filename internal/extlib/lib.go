// Package extlib is a tiny in-module library used to prove that
// external-package annotation resolution is generic (not stdlib-only).
package extlib

// Handle is a stand-in receiver type for method-contract tests.
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
