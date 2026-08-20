package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Diagnostic Protocol v1 conformance harness.
//
// These tests are the TDD driver for `gon check --json`. Against current main
// (no --json support, exit-3 injection absent, missing-file exit is 1 not 2)
// the suite is expected to be RED. Do not weaken assertions to make them green
// without implementing the protocol.
//
// Spec: docs/diagnostic-protocol-v1.md
// Fixtures: testdata/diagnostic-protocol/
//
// Expected RED gaps on pre-json main (implementation checklist):
//
//	1. accept `check --json <file>` (flag, not filename)
//	2. emit Protocol v1 envelope on stdout (schemaVersion + diagnostics)
//	3. map internal diagnostics → required fields + absolute file + range
//	4. zero-based UTF-8 byte columns (before_token.gon: nil at column 27)
//	5. symlink path reported as supplied path (no implicit realpath)
//	6. missing input → exit 2 (today: exit 1 on ReadFile error)
//	7. GON_TEST_INJECT_FAILURE → exit 3 (test-only; no public flag)
//	8. multi-file inputs (optional stretch; fixtures exist)
//
// PASS today (regression anchors):
//	- TestProtocol_HumanCheck_StillWorksWithoutJSON
//	- TestProtocol_MalformedInvocation_Exit2 (coincidental until flag parsing lands)

const protocolSchemaVersion = 1

// protocolEnvelope is the minimal JSON shape from Protocol v1.
type protocolEnvelope struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Diagnostics   []protocolDiagnostic `json:"diagnostics"`
}

type protocolDiagnostic struct {
	Code               string            `json:"code"`
	Severity           string            `json:"severity"`
	Message            string            `json:"message"`
	Source             string            `json:"source"`
	File               string            `json:"file"`
	Range              protocolRange     `json:"range"`
	RelatedInformation []protocolRelated `json:"relatedInformation"`
}

type protocolRange struct {
	Start protocolPos `json:"start"`
	End   protocolPos `json:"end"`
}

type protocolPos struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type protocolRelated struct {
	Message  string      `json:"message"`
	Location protocolLoc `json:"location"`
}

type protocolLoc struct {
	File  string        `json:"file"`
	Range protocolRange `json:"range"`
}

func protocolFixtureRoot(t *testing.T) string {
	t.Helper()
	// Tests run with cwd = package dir (cmd/gon).
	root := filepath.Join("..", "..", "testdata", "diagnostic-protocol")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs fixture root: %v", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		t.Fatalf("fixture root missing: %s (%v)", abs, err)
	}
	return abs
}

// captureRun redirects os.Stdout and os.Stderr while invoking run.
func captureRun(args []string) (exit int, stdout, stderr string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	or, ow, _ := os.Pipe()
	er, ew, _ := os.Pipe()
	os.Stdout, os.Stderr = ow, ew

	exit = run(args)

	_ = ow.Close()
	_ = ew.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var outBuf, errBuf bytes.Buffer
	_, _ = io.Copy(&outBuf, or)
	_, _ = io.Copy(&errBuf, er)
	_ = or.Close()
	_ = er.Close()
	return exit, outBuf.String(), errBuf.String()
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs %s: %v", p, err)
	}
	return a
}

// decodeProtocolEnvelope requires valid Protocol v1 JSON on stdout.
func decodeProtocolEnvelope(t *testing.T, stdout string) protocolEnvelope {
	t.Helper()
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		t.Fatalf("expected Protocol v1 JSON on stdout; got empty output")
	}
	var env protocolEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not valid Protocol v1 JSON: %v\nstdout:\n%s", err, stdout)
	}
	return env
}

func assertEnvelopeBasics(t *testing.T, env protocolEnvelope) {
	t.Helper()
	if env.SchemaVersion != protocolSchemaVersion {
		t.Errorf("schemaVersion: want %d, got %d", protocolSchemaVersion, env.SchemaVersion)
	}
	if env.Diagnostics == nil {
		t.Errorf("diagnostics: must be present (got nil); encoder must emit []")
	}
}

func assertDiagnosticShape(t *testing.T, d protocolDiagnostic, idx int) {
	t.Helper()
	prefix := "diagnostics[" + itoa(idx) + "]"
	if d.Code == "" {
		t.Errorf("%s.code: required, got empty", prefix)
	}
	if d.Severity != "error" && d.Severity != "warning" {
		t.Errorf("%s.severity: want error|warning, got %q", prefix, d.Severity)
	}
	if d.Message == "" {
		t.Errorf("%s.message: required, got empty", prefix)
	}
	if d.Source != "gon-check" && d.Source != "gon-vet" {
		t.Errorf("%s.source: want gon-check|gon-vet, got %q", prefix, d.Source)
	}
	if d.File == "" {
		t.Errorf("%s.file: required, got empty", prefix)
	}
	if !filepath.IsAbs(d.File) {
		t.Errorf("%s.file: must be absolute, got %q", prefix, d.File)
	}
	_ = d.Range
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func hasErrorDiag(env protocolEnvelope) bool {
	for _, d := range env.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func TestProtocol_Clean_Exit0_EmptyDiagnostics(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "clean", "ok.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 0 {
		t.Errorf("exit: want 0, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	if len(env.Diagnostics) != 0 {
		t.Errorf("diagnostics: want empty, got %d entries", len(env.Diagnostics))
	}
}

func TestProtocol_GN001_Exit1_Shape(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "errors", "gn001_assign.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("exit: want 1, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	if !hasErrorDiag(env) {
		t.Errorf("expected at least one error diagnostic")
	}
	found := false
	for i, d := range env.Diagnostics {
		assertDiagnosticShape(t, d, i)
		if d.Code == "GN001" && d.Severity == "error" {
			found = true
			if d.File != path {
				if filepath.Clean(d.File) != filepath.Clean(path) {
					t.Errorf("GN001 file: want %q, got %q", path, d.File)
				}
			}
		}
	}
	if !found {
		t.Errorf("expected a GN001 error diagnostic, got: %+v", env.Diagnostics)
	}
}

func TestProtocol_GN002_Exit1_Shape(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "errors", "gn002_struct.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("exit: want 1, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	found := false
	for i, d := range env.Diagnostics {
		assertDiagnosticShape(t, d, i)
		if d.Code == "GN002" && d.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a GN002 error diagnostic, got: %+v", env.Diagnostics)
	}
}

func TestProtocol_GW001_Exit0_WarningOnly(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "warnings", "gw001_compare.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 0 {
		t.Errorf("exit: want 0 (warnings do not fail), got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	if hasErrorDiag(env) {
		t.Errorf("warning-only fixture must not produce error diagnostics")
	}
	found := false
	for i, d := range env.Diagnostics {
		assertDiagnosticShape(t, d, i)
		if d.Code == "GW001" && d.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a GW001 warning diagnostic, got: %+v", env.Diagnostics)
	}
}

const beforeTokenNilLine = 3
const beforeTokenNilColumn = 27

func TestProtocol_UTF8_BeforeToken_ByteOffset(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "positions", "utf8", "before_token.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("exit: want 1, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)

	var gn001 *protocolDiagnostic
	for i := range env.Diagnostics {
		d := &env.Diagnostics[i]
		assertDiagnosticShape(t, *d, i)
		if d.Code == "GN001" {
			gn001 = d
			break
		}
	}
	if gn001 == nil {
		t.Fatalf("expected GN001 diagnostic for nil assignment")
	}

	if gn001.Range.Start.Line != beforeTokenNilLine {
		t.Errorf("range.start.line: want %d (0-based), got %d", beforeTokenNilLine, gn001.Range.Start.Line)
	}
	if gn001.Range.Start.Column != beforeTokenNilColumn {
		t.Errorf("range.start.column: want %d (UTF-8 byte offset of nil), got %d — "+
			"this distinguishes byte offset from rune/UTF-16 counts",
			beforeTokenNilColumn, gn001.Range.Start.Column)
	}
	if gn001.Range.End.Line < gn001.Range.Start.Line ||
		(gn001.Range.End.Line == gn001.Range.Start.Line && gn001.Range.End.Column <= gn001.Range.Start.Column) {
		t.Errorf("range.end must be exclusive and after start; got start=%+v end=%+v",
			gn001.Range.Start, gn001.Range.End)
	}
}

func TestProtocol_Symlink_PathNotResolved(t *testing.T) {
	root := protocolFixtureRoot(t)
	dir := t.TempDir()

	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetSrc, err := os.ReadFile(filepath.Join(root, "paths", "symlink", "real", "target.gon"))
	if err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(realDir, "target.gon")
	if err := os.WriteFile(targetPath, targetSrc, 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "via_link.gon")
	if err := os.Symlink(filepath.Join("real", "target.gon"), linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	linkAbs := mustAbs(t, linkPath)

	exit, stdout, stderr := captureRun([]string{"check", "--json", linkAbs})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("exit: want 1, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	if len(env.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for nil assignment via symlink")
	}
	for i, d := range env.Diagnostics {
		assertDiagnosticShape(t, d, i)
		got := filepath.Clean(d.File)
		want := filepath.Clean(linkAbs)
		if got != want {
			phys, _ := filepath.EvalSymlinks(linkAbs)
			if phys != "" && filepath.Clean(d.File) == filepath.Clean(phys) {
				t.Errorf("diagnostics[%d].file: resolved to physical path %q; protocol forbids implicit realpath", i, d.File)
			} else {
				t.Errorf("diagnostics[%d].file: want supplied path %q, got %q", i, want, d.File)
			}
		}
	}
}

func TestProtocol_MissingInput_Exit2(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.gon")
	exit, stdout, stderr := captureRun([]string{"check", missing})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 2 {
		t.Errorf("exit: want 2 for nonexistent input, got %d (current main returns 1 on ReadFile error)", exit)
	}
	_ = stdout
}

func TestProtocol_MalformedInvocation_Exit2(t *testing.T) {
	exit, stdout, stderr := captureRun([]string{"check", "--json"})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
	if exit != 2 {
		t.Errorf("exit: want 2 for malformed invocation, got %d", exit)
	}
	_, _ = stdout, stderr
}

func TestProtocol_InternalFailure_Exit3(t *testing.T) {
	t.Setenv("GON_TEST_INJECT_FAILURE", "1")
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "clean", "ok.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 3 {
		t.Errorf("exit: want 3 for controlled internal failure, got %d "+
			"(implement test-only GON_TEST_INJECT_FAILURE; do not expose a public CLI flag)", exit)
	}
	_, _ = stdout, stderr
}

func TestProtocol_MultipleFiles_WhenSupported(t *testing.T) {
	root := protocolFixtureRoot(t)
	a := mustAbs(t, filepath.Join(root, "multiple-files", "a.gon"))
	b := mustAbs(t, filepath.Join(root, "multiple-files", "b.gon"))

	exit, stdout, stderr := captureRun([]string{"check", "--json", a, b})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("exit: want 1 when any input has errors, got %d", exit)
	}
	env := decodeProtocolEnvelope(t, stdout)
	assertEnvelopeBasics(t, env)
	codes := map[string]bool{}
	files := map[string]bool{}
	for i, d := range env.Diagnostics {
		assertDiagnosticShape(t, d, i)
		codes[d.Code] = true
		files[filepath.Clean(d.File)] = true
	}
	if !codes["GN001"] {
		t.Errorf("expected GN001 from multi-file inputs")
	}
	if !files[filepath.Clean(a)] && !files[filepath.Clean(b)] {
		t.Errorf("expected diagnostics referencing at least one of the input files; got %v", files)
	}
}

func TestProtocol_HumanCheck_StillWorksWithoutJSON(t *testing.T) {
	root := protocolFixtureRoot(t)
	path := mustAbs(t, filepath.Join(root, "errors", "gn001_assign.gon"))

	exit, stdout, stderr := captureRun([]string{"check", path})
	t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)

	if exit != 1 {
		t.Errorf("human check exit: want 1, got %d", exit)
	}
	if strings.TrimSpace(stderr) == "" && strings.TrimSpace(stdout) == "" {
		t.Errorf("human check: expected diagnostic text on stderr or stdout")
	}
	if strings.Contains(stdout, `"schemaVersion"`) {
		t.Errorf("human check must not emit Protocol JSON without --json")
	}
}
