package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const messy = "package main\nfunc  main( ){\nprintln( \"hi\" )}\n"

func TestTranspileFormatsWithGofmt(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	out, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "func main() {") || !strings.Contains(string(out), `println("hi")`) {
		t.Fatalf("source was not gofmt-formatted:\n%s", out)
	}
}

// When gofmt is unavailable the clean source is returned unchanged, with no
// error (documented behaviour).
func TestTranspileFallsBackToInputWithoutGofmt(t *testing.T) {
	t.Setenv("PATH", "")
	out, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatalf("a missing gofmt must not be an error: %v", err)
	}
	if string(out) != messy {
		t.Fatalf("expected the input back unchanged, got:\n%s", out)
	}
}

func TestTranspileToFileWritesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.go")
	if err := TranspileToFile([]byte(messy), path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "package main") {
		t.Fatalf("output file not written correctly:\n%s", got)
	}
}

func TestTranspileToFileReportsWriteError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "out.go")
	if err := TranspileToFile([]byte(messy), bad); err == nil {
		t.Fatal("writing into a missing directory must fail")
	}
}
