package main

import (
	"strings"
	"testing"
)

// All-or-nothing (Protocol v1 §7): one invocation writes one envelope with
// every operand's diagnostics, or, when an operand fails before emission, no
// envelope at all. Diagnostics already collected from earlier operands are not
// sent. These pin that the exit-code reclassification did not add a partial
// envelope on any of the failing paths.

// R2: file mode, a GN001 file then a file that fails the pre-check parse.
func TestAllOrNothingFileParseFailureAfterDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{"good.gon": exitGN001, "bad.gon": "package p\n\nfunc g( {\n"})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "good.gon", "bad.gon"})
	if exit != 1 || out != "" || !strings.Contains(errOut, "gon: parse error:") {
		t.Fatalf("exit %d (want 1), stdout %q (want none)\n%s", exit, out, errOut)
	}
}

// R7: `./...`, package a has GN001, package b cannot be loaded.
func TestAllOrNothingPatternLoadFailureAfterDiagnostics(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"a/a.gon": "package a\n\nfunc f() !*int {\n\treturn nil\n}\n",
		"b/b.gon": "//go:build ignore\n\npackage b\n",
		"b/c.gon": "package b\n",
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./..."})
	if exit != 2 || out != "" || !strings.Contains(errOut, "excluded by build constraints: b/b.gon") {
		t.Fatalf("exit %d (want 2), stdout %q (want none)\n%s", exit, out, errOut)
	}
	if strings.Contains(errOut, "GN001") {
		t.Fatalf("--json does not print a's diagnostics anywhere:\n%s", errOut)
	}
}

// A file operand with GN001, then a directory that cannot be loaded.
func TestAllOrNothingFileThenFailingDirectory(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"good.gon": exitGN001,
		"d/a.gon":  exitIgnored,
		"d/b.gon":  exitOK,
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "good.gon", "./d"})
	if exit != 2 || out != "" {
		t.Fatalf("exit %d (want 2), stdout %q (want none)\n%s", exit, out, errOut)
	}
}

// The other order: a failing directory first stops before the file operand.
func TestAllOrNothingFailingDirectoryThenFile(t *testing.T) {
	root := pkgModule(t, map[string]string{
		"good.gon": exitGN001,
		"d/a.gon":  exitIgnored,
		"d/b.gon":  exitOK,
	})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./d", "good.gon"})
	if exit != 2 || out != "" {
		t.Fatalf("exit %d (want 2), stdout %q (want none)\n%s", exit, out, errOut)
	}
}

// Contrast: every operand checked, a package-mode loader error (R13) and a
// GN001 elsewhere are both in the one envelope.
func TestAllOrNothingCompletedRunHasOneEnvelope(t *testing.T) {
	root := pkgModule(t, map[string]string{"good.gon": exitGN001, "p/a.gon": exitNoPkg})
	chdir(t, root)
	exit, out, errOut := captureRun([]string{"check", "--json", "./p", "good.gon"})
	if exit != 1 {
		t.Fatalf("exit %d, want 1\n%s", exit, errOut)
	}
	env := decodeProtocolEnvelope(t, out)
	if len(env.Diagnostics) != 1 || env.Diagnostics[0].Code != "GN001" {
		t.Fatalf("want good.gon's GN001: %+v", env.Diagnostics)
	}
	if strings.Count(out, "schemaVersion") != 1 {
		t.Fatalf("exactly one envelope:\n%s", out)
	}
}
