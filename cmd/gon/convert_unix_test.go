//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The mode of a converted file is the mode it had, whatever the umask of the process.
func TestConvertWriteKeepsModeUnderARestrictiveUmask(t *testing.T) {
	root := wtModule(t)
	if err := os.Chmod(filepath.Join(root, "b.gon"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	if exit, _, errOut := captureRun([]string{"convert", "--write", "./..."}); exit != 0 {
		t.Fatalf("exit %d\n%s", exit, errOut)
	}
	if p := wtPerm(t, filepath.Join(root, "b.gon")); p != 0o755 {
		t.Errorf("b.gon mode = %o, want 755", p)
	}
}
