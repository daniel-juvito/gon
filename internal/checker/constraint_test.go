// MalformedConstraint: go/build's verdict on the //go:build line of one file
// operand, by its contents only.
package checker

import (
	"runtime"
	"strings"
	"testing"
)

const constraintBody = "package main\n\nfunc main() {}\n"

// The verdict for every shape measured against `go list -e` / `go build` on
// the same file named explicitly (go 1.21, 1.23, 1.24, 1.27: no difference).
func TestMalformedConstraintVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		reason    string // "" means not malformed
	}{
		{"no constraint", constraintBody, ""},
		{"valid, matching", "//go:build linux\n\n" + constraintBody, ""},
		{"valid, not matching", "//go:build windows\n\n" + constraintBody, ""},
		{"valid, ignore", "//go:build ignore\n\n" + constraintBody, ""},
		{"valid, complex", "//go:build (linux && amd64) || !windows\n\n" + constraintBody, ""},
		{"missing paren", "//go:build (linux\n\n" + constraintBody, "parsing //go:build line: missing close paren"},
		{"trailing &&", "//go:build linux &&\n\n" + constraintBody, "parsing //go:build line: unexpected end of expression"},
		{"two //go:build lines", "//go:build ignore\n//go:build linux\n\n" + constraintBody, "multiple //go:build comments"},
		{"empty expression", "//go:build\n\n" + constraintBody, "parsing //go:build line: unexpected end of expression"},
		{"two terms", "//go:build linux windows\n\n" + constraintBody, "parsing //go:build line: unexpected token windows"},
		{"leading &&", "//go:build && linux\n\n" + constraintBody, "parsing //go:build line: unexpected token &&"},
		{"lone !", "//go:build !\n\n" + constraintBody, "parsing //go:build line: unexpected end of expression"},
		{"extra paren", "//go:build linux)\n\n" + constraintBody, "parsing //go:build line: unexpected token )"},
		{"single &", "//go:build linux & windows\n\n" + constraintBody, "parsing //go:build line: invalid syntax at &"},
		{"bad character", "//go:build linux$\n\n" + constraintBody, "parsing //go:build line: invalid syntax at $"},
		{"empty parentheses", "//go:build ()\n\n" + constraintBody, "parsing //go:build line: unexpected token )"},
		{"double ||", "//go:build linux || || windows\n\n" + constraintBody, "parsing //go:build line: unexpected token ||"},
		{"after a license comment", "// Copyright\n\n//go:build (linux\n\n" + constraintBody, "parsing //go:build line: missing close paren"},
		{"CRLF", "//go:build (linux\r\n\r\n" + constraintBody, "parsing //go:build line: missing close paren"},
		{"tab after //go:build", "//go:build\t(linux\n\n" + constraintBody, "parsing //go:build line: missing close paren"},
		{"byte order mark", "\xef\xbb\xbf//go:build (linux\n\n" + constraintBody, "parsing //go:build line: missing close paren"},
		// Not malformed constraints: the go command accepts them, so does this.
		{"old // +build only", "// +build linux\n\n" + constraintBody, ""},
		{"odd // +build", "// +build linux,,\n\n" + constraintBody, ""},
		{"// +build and //go:build disagree", "//go:build linux\n// +build windows\n\n" + constraintBody, ""},
		{"misplaced after package (out of scope)", "package main\n\n//go:build (linux\n\nfunc main() {}\n", ""},
		{"block comment form", "/*go:build (linux*/\n\n" + constraintBody, ""},
		{"no space after //go:build", "//go:build(linux\n\n" + constraintBody, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, bad := MalformedConstraint([]byte(tc.src))
			if bad != (tc.reason != "") {
				t.Fatalf("malformed = %v (%q), want %v", bad, reason, tc.reason != "")
			}
			if bad && reason != tc.reason {
				t.Fatalf("reason = %q, want %q", reason, tc.reason)
			}
			if bad && strings.Contains(reason, operandName) {
				t.Fatalf("the reason must not carry the file name: %q", reason)
			}
		})
	}
}

// The verdict is independent of the operand's name because the function has no
// name to take: go/build asked about the real name would not even read the
// line of `a_windows.go` on linux, which `go vet a_windows.go` still rejects.
// Which names go/build drops without reading depends on the platform running
// the test, so the expectation is computed from it: a name starting with `_`
// or `.`, and a GOOS or GOARCH suffix that is not this platform's.
func TestMalformedConstraintDoesNotDependOnTheOperandName(t *testing.T) {
	otherOS := "windows"
	if runtime.GOOS == "windows" {
		otherOS = "plan9"
	}
	otherArch := "arm64"
	if runtime.GOARCH == "arm64" {
		otherArch = "amd64"
	}
	for _, tc := range []struct {
		name          string
		ignoredByName bool // go/build drops it before reading the line, on this platform
	}{
		{"a.go", false},
		{"a_test.go", false},
		{"a_" + runtime.GOOS + ".go", false},
		{"a_" + runtime.GOARCH + ".go", false},
		{"a_" + runtime.GOOS + "_" + runtime.GOARCH + ".go", false},
		{"_a.go", true},
		{".a.go", true},
		{"a_" + otherOS + ".go", true},
		{"a_" + otherArch + ".go", true},
		{"a_" + otherOS + "_" + runtime.GOARCH + ".go", true},
	} {
		src := []byte("//go:build (linux\n\n" + constraintBody)
		_, err := overlayContext(map[string][]byte{tc.name: src}).MatchFile(".", tc.name)
		if ignoredByName := err == nil; ignoredByName != tc.ignoredByName {
			t.Errorf("MatchFile(%q): dropped by name = %v, want %v (the premise of the neutral name on %s/%s)",
				tc.name, ignoredByName, tc.ignoredByName, runtime.GOOS, runtime.GOARCH)
		}
		// Whatever go/build does with the real name, the verdict on the
		// contents is the same.
		if _, bad := MalformedConstraint(src); !bad {
			t.Errorf("%q: malformed whatever the name", tc.name)
		}
	}
}

// Only an error counts: a valid constraint that does not match is not a
// verdict, and a file with a syntax error after the header is not this
// function's business.
func TestMalformedConstraintIgnoresEverythingButTheConstraint(t *testing.T) {
	for _, src := range []string{
		"//go:build windows\n\npackage main\n",
		"package main\n\nfunc broken( {\n",
		"pakage main\n",
		"",
	} {
		if reason, bad := MalformedConstraint([]byte(src)); bad {
			t.Errorf("%q: malformed (%s), want not", src, reason)
		}
	}
}
