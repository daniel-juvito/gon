package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// End-to-end CLI contract tests for v1.0 release hardening.
// Diagnostics → stderr; exit codes as documented; transpile emits valid Go.

func writeTempGon(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLICheckClean(t *testing.T) {
	dir := t.TempDir()
	path := writeTempGon(t, dir, "ok.gon", `package main
func main() {
	n := 1
	var x !*int = &n
	_ = x
}
`)
	code := run([]string{"check", path})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCLICheckGN001(t *testing.T) {
	dir := t.TempDir()
	path := writeTempGon(t, dir, "bad.gon", `package main
func f(x !*int) {}
func main() {
	f(nil)
}
`)
	// Capture is done by the process; we only assert exit code here.
	// Full message format is covered by checker unit tests.
	code := run([]string{"check", path})
	if code != 1 {
		t.Fatalf("expected exit 1 on GN001, got %d", code)
	}
}

func TestCLICheckWarningDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	path := writeTempGon(t, dir, "warn.gon", `package main
func f(x !*int) {
	if x == nil {}
}
`)
	code := run([]string{"check", path})
	if code != 0 {
		t.Fatalf("GW001 alone must exit 0, got %d", code)
	}
}

func TestCLIUsageExit2(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("no args: expected 2, got %d", code)
	}
	if code := run([]string{"check"}); code != 2 {
		t.Fatalf("missing file: expected 2, got %d", code)
	}
	if code := run([]string{"nope", "x.gon"}); code != 2 {
		t.Fatalf("unknown command: expected 2, got %d", code)
	}
}

func TestCLIVersion(t *testing.T) {
	if code := run([]string{"version"}); code != 0 {
		t.Fatalf("version: expected 0, got %d", code)
	}
}

func TestCLITranspileEmitsGo(t *testing.T) {
	dir := t.TempDir()
	// Run from temp dir so relative output lands next to input.
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	src := `package main
func f(x !*int) {}
func main() {
	n := 1
	f(&n)
}
`
	path := writeTempGon(t, dir, "sample.gon", src)
	code := run([]string{"transpile", path})
	if code != 0 {
		t.Fatalf("transpile: expected 0, got %d", code)
	}
	outPath := strings.TrimSuffix(path, ".gon") + ".go"
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected .go output: %v", err)
	}
	// ! must be stripped.
	if strings.Contains(string(data), "!*int") {
		t.Fatalf("transpiled source still contains !: %s", data)
	}
	if !strings.Contains(string(data), "func f(x *int)") {
		t.Fatalf("expected clean signature, got:\n%s", data)
	}
}

func TestCLIRejectsNonGonExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"check", path}); code != 2 {
		t.Fatalf("non-.gon extension: expected 2, got %d", code)
	}
}

func TestCLIFlowInsensitiveAssignmentAllowed(t *testing.T) {
	// Documents the v1 non-guarantee: non-literal assignments are accepted.
	dir := t.TempDir()
	path := writeTempGon(t, dir, "flow.gon", `package main
func get() *int { return nil }
func main() {
	var x !*int = get()
	_ = x
}
`)
	if code := run([]string{"check", path}); code != 0 {
		t.Fatalf("flow-insensitive assignment must be accepted, got exit %d", code)
	}
}
