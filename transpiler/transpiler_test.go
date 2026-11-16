package transpiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const messy = "package main\nfunc  main( ){\nprintln( \"hi\" )}\n"

func TestTranspileFormatsInProcess(t *testing.T) {
	out, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "func main() {") || !strings.Contains(string(out), `println("hi")`) {
		t.Fatalf("source was not formatted:\n%s", out)
	}
}

// v1.12 (T7): the output must not depend on PATH. Before, a missing or older
// gofmt made Transpile return the UNFORMATTED source.
func TestTranspileDoesNotDependOnPATH(t *testing.T) {
	want, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	got, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatalf("a missing gofmt must not matter: %v", err)
	}
	if string(got) != string(want) || string(got) == messy {
		t.Fatalf("output changed with PATH or was not formatted:\n%s", got)
	}
}

// Where a gofmt is available it produces the same bytes as the in-process
// formatter.
func TestTranspileMatchesGofmt(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH")
	}
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(messy)
	want, err := cmd.Output()
	if err != nil {
		t.Skipf("this gofmt cannot format the fixture: %v", err)
	}
	got, err := Transpile([]byte(messy))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("go/format and gofmt differ:\n%s\n---\n%s", got, want)
	}
}

// Source that does not parse is an error, not passed through unformatted.
func TestTranspileRejectsUnparsableSource(t *testing.T) {
	if _, err := Transpile([]byte("package main\nfunc main( {\n")); err == nil {
		t.Fatal("unparsable source must be an error")
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
