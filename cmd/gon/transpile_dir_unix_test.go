//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask sets the process umask for the duration of the test. Tests of this
// package do not run in parallel, so the process-wide change is safe.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

func emittedMode(t *testing.T, mask int) os.FileMode {
	t.Helper()
	withUmask(t, mask)
	root := pkgModule(t, map[string]string{"a.gon": tdTypes})
	chdir(t, root)
	if exit, _, errOut := captureRun([]string{"transpile", "."}); exit != 0 {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
	fi, err := os.Stat(filepath.Join(root, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// The emitted file's mode is 0644 &^ umask, as os.WriteFile gave before v1.12's
// directory form: a private umask must not be widened to world-readable. Found in
// the audit (a forced chmod 0644 ignored the umask).
func TestTranspileDirModeFollowsTheUmask(t *testing.T) {
	if got := emittedMode(t, 0o022); got != 0o644 {
		t.Errorf("umask 022: mode %v, want 0644", got)
	}
	if got := emittedMode(t, 0o077); got != 0o600 {
		t.Errorf("umask 077: mode %v, want 0600", got)
	}
	if got := emittedMode(t, 0o027); got != 0o640 {
		t.Errorf("umask 027: mode %v, want 0640", got)
	}
}
