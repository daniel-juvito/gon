// Package goenv holds small fixes to the process's view of the Go toolchain.
package goenv

import (
	"go/build"
	"os"
	"os/exec"
	"strings"
)

// EnsureGOROOT makes go/build — and with it importer.Default(), which Gon uses
// to type-check a file that has no enclosing module — find the standard
// library even when the binary was built with -trimpath.
//
// A -trimpath binary has an empty runtime.GOROOT() and build.Default.GOROOT,
// and the official release binaries are built that way. Without a GOROOT the
// standalone importer fails with "could not import fmt … ($GOROOT not set)".
// Files inside a module are unaffected (they go through `go list`), as is a
// process that already has GOROOT in its environment.
//
// The value is taken, in order, from build.Default, the GOROOT environment
// variable, then `go env GOROOT`. If none yields a path nothing changes.
func EnsureGOROOT() {
	if build.Default.GOROOT != "" {
		return
	}
	if g := os.Getenv("GOROOT"); g != "" {
		build.Default.GOROOT = g
		return
	}
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return
	}
	if g := strings.TrimSpace(string(out)); g != "" {
		build.Default.GOROOT = g
	}
}
