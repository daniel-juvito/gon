package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The Go installed on a USER'S machine is not the Go that built Gon. This test
// runs a release-style binary (CGO off, -trimpath -s -w) with the user's `go`
// forced to the version named by GON_USER_GO, and checks that the diagnostics
// do not depend on it. CI runs it once per version of the user-Go matrix
// (.github/workflows/ci.yml); without GON_USER_GO it is skipped.
//
// Two shapes are covered because they take different code paths: a file inside
// a module (go/packages, which shells out to the user's `go list`) and a file
// outside any module (the standalone importer, which needs GOROOT).
func TestUserGoVersion(t *testing.T) {
	ver := os.Getenv("GON_USER_GO")
	if ver == "" {
		t.Skip("set GON_USER_GO (e.g. 1.21.13) to run")
	}

	bin := filepath.Join(t.TempDir(), "gon-release")
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", bin, ".")
	build.Env = append(userGoEnv(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the release-style binary: %v\n%s", err, out)
	}

	t.Run("inside a module", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "go.mod"), "module usergo\n\ngo 1.21\n")
		copyFixture(t, "check.gon", filepath.Join(dir, "check.gon"))

		out, code := runGon(t, bin, dir, ver, "check", "--json", "check.gon")
		if code != 1 {
			t.Fatalf("exit code = %d, want 1\n%s", code, out)
		}
		if strings.Contains(out, "Go type error") {
			t.Fatalf("unexpected Go type error:\n%s", out)
		}
		got := diagnosticCodes(t, out)
		want := []string{"GN002", "GN003"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("diagnostics = %v, want %v\n%s", got, want, out)
		}
	})

	t.Run("outside any module", func(t *testing.T) {
		// os.MkdirTemp under the system temp dir: no go.mod above it.
		dir, err := os.MkdirTemp("", "gon-usergo-nomod-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		copyFixture(t, "hello.gon", filepath.Join(dir, "hello.gon"))

		out, code := runGon(t, bin, dir, ver, "check", "hello.gon")
		if code != 0 || !strings.Contains(out, "ok") {
			t.Fatalf("a stdlib-only file outside a module must be ok: exit %d\n%s", code, out)
		}
	})
}

// userGoEnv is the test process environment without GOROOT/GOFLAGS, so the
// child behaves like an ordinary user shell.
func userGoEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GOROOT=") || strings.HasPrefix(kv, "GOFLAGS=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

func runGon(t *testing.T, bin, dir, userGo string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(userGoEnv(), "GOTOOLCHAIN=go"+userGo)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running gon: %v", err)
		}
		code = ee.ExitCode()
	}
	return string(out), code
}

func copyFixture(t *testing.T, name, dst string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "usergo", name))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dst, string(b))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// diagnosticCodes returns the sorted diagnostic codes of a Diagnostic
// Protocol v1 document (the --json output).
func diagnosticCodes(t *testing.T, out string) []string {
	t.Helper()
	i := strings.Index(out, "{")
	if i < 0 {
		t.Fatalf("no JSON in output:\n%s", out)
	}
	var doc struct {
		SchemaVersion int `json:"schemaVersion"`
		Diagnostics   []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out[i:]), &doc); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if doc.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", doc.SchemaVersion)
	}
	var codes []string
	for _, d := range doc.Diagnostics {
		codes = append(codes, d.Code)
	}
	sort.Strings(codes)
	return codes
}
