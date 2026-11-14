package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Release binaries are built with -trimpath, which leaves go/build's GOROOT
// empty. A file whose folder has no go.mod is then type-checked with the
// standalone importer, which could not find the standard library:
//
//	gon: Go type error: could not import fmt ... ($GOROOT not set)
//
// This builds a real -trimpath binary and runs it on such a file with GOROOT
// unset in its environment, exactly as a user with the official release does.
func TestTrimpathBinaryFindsStdlibWithoutModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	bin := filepath.Join(t.TempDir(), "gon-trimpath")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Env = cleanEnv(os.Environ())
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -trimpath: %v\n%s", err, out)
	}

	// A directory with no go.mod anywhere above it.
	dir, err := os.MkdirTemp("", "gon-nomod-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	src := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n"
	file := filepath.Join(dir, "hello.gon")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	run := exec.Command(bin, "check", file)
	run.Dir = dir
	run.Env = cleanEnv(os.Environ())
	out, err := run.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok") {
		t.Fatalf("trimpath binary must check a stdlib-only file outside a module: err=%v\n%s", err, out)
	}
}

// cleanEnv drops GOROOT (and GOFLAGS, which could add -mod/-trimpath
// surprises) so the child behaves like an ordinary user shell.
func cleanEnv(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOROOT=") || strings.HasPrefix(kv, "GOFLAGS=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
