package goenv

import (
	"go/build"
	"os/exec"
	"strings"
	"testing"
)

func TestEnsureGOROOTFillsEmptyDefault(t *testing.T) {
	old := build.Default.GOROOT
	t.Cleanup(func() { build.Default.GOROOT = old })
	t.Setenv("GOROOT", "")

	build.Default.GOROOT = ""
	EnsureGOROOT()

	want, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	if got := build.Default.GOROOT; got == "" || got != strings.TrimSpace(string(want)) {
		t.Fatalf("GOROOT = %q, want %q", got, strings.TrimSpace(string(want)))
	}
}

func TestEnsureGOROOTKeepsExistingValue(t *testing.T) {
	old := build.Default.GOROOT
	t.Cleanup(func() { build.Default.GOROOT = old })

	build.Default.GOROOT = "/already/set"
	EnsureGOROOT()
	if build.Default.GOROOT != "/already/set" {
		t.Fatalf("an existing GOROOT must not be replaced, got %q", build.Default.GOROOT)
	}
}

func TestEnsureGOROOTPrefersEnvironment(t *testing.T) {
	old := build.Default.GOROOT
	t.Cleanup(func() { build.Default.GOROOT = old })
	t.Setenv("GOROOT", "/from/env")

	build.Default.GOROOT = ""
	EnsureGOROOT()
	if build.Default.GOROOT != "/from/env" {
		t.Fatalf("GOROOT env must win over `go env`, got %q", build.Default.GOROOT)
	}
}
