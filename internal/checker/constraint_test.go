// MalformedConstraint: go/build's verdict on the //go:build line of one file
// operand, by its contents only.
package checker

import (
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
// line of `a_windows.go`, which `go vet a_windows.go` still rejects.
func TestMalformedConstraintDoesNotDependOnTheOperandName(t *testing.T) {
	src := []byte("//go:build (linux\n\n" + constraintBody)
	for _, name := range []string{"a.go", "a_linux.go", "a_test.go", "_a.go", ".a.go", "a_windows.go", "a_plan9.go"} {
		overlay := map[string][]byte{name: src}
		_, err := overlayContext(overlay).MatchFile(".", name)
		ignoredByName := err == nil // go/build never parsed the line
		wantIgnored := name == "_a.go" || name == ".a.go" || name == "a_windows.go" || name == "a_plan9.go"
		if ignoredByName != wantIgnored {
			t.Errorf("MatchFile(%q): ignored by name = %v, want %v (the premise of the neutral name)", name, ignoredByName, wantIgnored)
		}
	}
	if _, bad := MalformedConstraint(src); !bad {
		t.Fatal("malformed whatever the name")
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
