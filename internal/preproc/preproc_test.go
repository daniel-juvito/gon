package preproc

import (
	"bytes"
	"strings"
	"testing"
)

func TestMultiReturnLocalSignatures(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// substrings that must appear in clean (with ! stripped)
		wantClean []string
		// substrings that must NOT appear (the ! forms)
		wantGone []string
	}{
		{
			name: "first result non-nil",
			src: `package main
func Open() (!*string, error) {
	s := "x"
	return &s, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!*string"},
		},
		{
			name: "second result non-nil",
			src: `package main
func Open() (*string, !error) {
	return nil, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!error"},
		},
		{
			name: "both results non-nil",
			src: `package main
func Open() (!*string, !error) {
	s := "x"
	return &s, nil
}
`,
			wantClean: []string{"func Open() (*string, error)"},
			wantGone:  []string{"!*string", "!error"},
		},
		{
			name: "params and multi-return",
			src: `package main
func F(a !*int, b !*string) (!*int, error) {
	return a, nil
}
`,
			wantClean: []string{"func F(a *int, b *string) (*int, error)"},
			wantGone:  []string{"!*int", "!*string"},
		},
		{
			name: "unnamed params",
			src: `package main
func G(!*int, !*string) !*int {
	return nil
}
`,
			wantClean: []string{"func G(*int, *string) *int"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Process("t.gon", []byte(tc.src))
			clean := string(r.Clean)
			for _, w := range tc.wantClean {
				if !strings.Contains(clean, w) {
					t.Fatalf("clean missing %q:\n%s", w, clean)
				}
			}
			for _, g := range tc.wantGone {
				if strings.Contains(clean, g) {
					t.Fatalf("clean still contains %q:\n%s", g, clean)
				}
			}
			if len(r.NonNilOffsets) == 0 {
				t.Fatalf("expected some NonNilOffsets, clean:\n%s", clean)
			}
		})
	}
}

func TestUnaryNotNotStripped(t *testing.T) {
	src := `package main
func f(a bool, b bool) bool {
	return f(a, !b) || (!a) || !b
}
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	// Unary ! must remain.
	if !strings.Contains(clean, "!b") {
		t.Fatalf("unary !b was stripped:\n%s", clean)
	}
	if !strings.Contains(clean, "(!a)") {
		t.Fatalf("unary (!a) was stripped:\n%s", clean)
	}
}

func TestClassicStillWorks(t *testing.T) {
	src := `package main
type S struct { X !*int }
func f(x !*int) !*int { return x }
func (r !*S) M() {}
var y !*int
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	if strings.Contains(clean, "!") {
		// only unary would remain; there should be none here
		t.Fatalf("unexpected ! left in clean:\n%s", clean)
	}
	if !bytes.Contains(r.Clean, []byte("func f(x *int) *int")) {
		t.Fatalf("classic signature broken:\n%s", clean)
	}
}

func TestInterfaceMethodMultiReturn(t *testing.T) {
	src := `package main
type I interface {
	Open() (!*string, error)
}
`
	r := Process("t.gon", []byte(src))
	clean := string(r.Clean)
	if !strings.Contains(clean, "Open() (*string, error)") {
		t.Fatalf("interface method multi-return not stripped:\n%s", clean)
	}
	if strings.Contains(clean, "!*string") {
		t.Fatalf("! left in interface method:\n%s", clean)
	}
}
