package checker

import "testing"

// Rule C2 of the RFC on local backdoors (accepted 2026-11-25, strict): a bare return in a
// function that has a named result declared `!T` is GN002, whatever was assigned before.

func TestC2_BareReturnWithANamedBangResult(t *testing.T) {
	for name, body := range map[string]string{
		"nothing assigned":    `func f() (x !*T) { return }`,
		"assigned a source":   `func f() (x !*T) { x = nn(); return }`,
		"assigned on a path":  `func f(c bool) (x !*T) { if c { x = nn() }; return }`,
		"assigned in a defer": `func f() (x !*T) { defer func() { x = nn() }(); return }`,
		"second result":       `func f() (a *T, x !*T) { a = ord(); x = nn(); return }`,
		"blank name":          `func f() (_ !*T) { return }`,
		"method":              `type R struct{}; func (R) m() (x !*T) { x = nn(); return }`,
		"function literal":    `func f() { g := func() (x !*T) { x = nn(); return }; _ = g }`,
		"two bare returns":    `func f(c bool) (x !*T) { if c { return }; x = nn(); return }`,
		"after a literal":     `func f() (x !*T) { g := func() { return }; _ = g; x = nn(); return }`,
	} {
		t.Run(name, func(t *testing.T) {
			d := zvCheck(t, body)
			n := countCode(d, "GN002")
			want := 1
			if name == "two bare returns" {
				want = 2
			}
			if n != want {
				t.Fatalf("expected %d GN002, got %d: %v", want, n, d)
			}
		})
	}
}

func TestC2_SilentForms(t *testing.T) {
	for name, body := range map[string]string{
		"explicit return":          `func f() (x !*T) { x = nn(); return x }`,
		"explicit call":            `func f() (x !*T) { return nn() }`,
		"named but not bang":       `func f() (x *T) { x = ord(); return }`,
		"no results":               `func f() { return }`,
		"unnamed bang result":      `func f() !*T { return nn() }`,
		"literal inside, own rule": `func f() (x !*T) { g := func() { return }; _ = g; return nn() }`,
		"bare return of the outer": `func f() (x *T) { g := func() (y !*T) { return nn() }; _ = g; return }`,
	} {
		t.Run(name, func(t *testing.T) {
			zvWant(t, zvCheck(t, body), "GN002", 0)
		})
	}
}

func TestC2_MessageNamesTheResult(t *testing.T) {
	d := zvCheck(t, `func f() (a *T, found !*T) { return }`)
	zvWant(t, d, "GN002", 1)
	if !containsStr(d[0].Message, "named non-nil result found") {
		t.Errorf("message = %q", d[0].Message)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
