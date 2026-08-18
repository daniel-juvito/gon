package fmt

import (
	"strings"
	"testing"
)

func TestFormatPreservesBang(t *testing.T) {
	out, err := Format("t.gon", []byte("package main\nfunc f(x !*int) {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "!*int") {
		t.Fatalf("expected !*int, got:\n%s", out)
	}
}

func TestFormatIdempotentBang(t *testing.T) {
	src := []byte("package main\nfunc f(x !*int) {}\n")
	out1, err := Format("t.gon", src)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := Format("t.gon", out1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), "!*int") {
		t.Fatalf("second format lost !: %s", out2)
	}
}
