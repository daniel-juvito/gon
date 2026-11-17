// Construction invariants behind the checker's own `!` contracts (dogfood D5,
// Proposal A, P1–P3). The contracts are static: Gon proves that no `nil` is
// returned, passed or stored in their place in the .gon sources. These tests
// lock the runtime side the contracts rely on: every way the values are built
// produces a non-nil value.
package checker

import (
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
)

// --- P1: withPath and cloneGnaFile never return nil --------------------------

func TestWithPathNeverReturnsNil(t *testing.T) {
	// Nil receiver: a new trace holding only the segment.
	var none *ContractTrace
	got := none.withPath("a")
	if got == nil || len(got.Path) != 1 || got.Path[0] != "a" {
		t.Fatalf("nil receiver: got %+v", got)
	}

	// Non-nil receiver: a clone with the segment appended; the receiver is
	// unchanged.
	base := &ContractTrace{Origin: "o", Path: []string{"x"}, Declared: "!*int"}
	got = base.withPath("y")
	if got == nil || got == base {
		t.Fatalf("non-nil receiver: got %p (base %p)", got, base)
	}
	if got.Origin != "o" || got.Declared != "!*int" || len(got.Path) != 2 || got.Path[0] != "x" || got.Path[1] != "y" {
		t.Fatalf("non-nil receiver: got %+v", got)
	}
	if len(base.Path) != 1 {
		t.Fatalf("receiver modified: %+v", base)
	}
}

func TestCloneGnaFileNeverReturnsNil(t *testing.T) {
	for name, f := range map[string]*gna.File{
		"empty": {},
		"full": {
			Package:   "p",
			Functions: map[string]*gna.Signature{"F": {Params: []bool{true}, Results: []bool{false}}, "Nil": nil},
			Methods:   map[string]*gna.Signature{"T.M": {Results: []bool{true}}},
			Types:     map[string]*gna.TypeAnnotation{"T": {Fields: map[string]bool{"A": true}}, "U": nil},
		},
	} {
		t.Run(name, func(t *testing.T) {
			cp := cloneGnaFile(f)
			if cp == nil || cp == f {
				t.Fatalf("got %p (input %p)", cp, f)
			}
			if cp.Package != f.Package || len(cp.Functions) != len(f.Functions) ||
				len(cp.Methods) != len(f.Methods) || len(cp.Types) != len(f.Types) {
				t.Fatalf("copy differs: %+v vs %+v", cp, f)
			}
			// The flags are copied, not shared: clearing one in the copy leaves
			// the input as it was.
			if sig := cp.Functions["F"]; sig != nil {
				sig.Params[0] = false
				if !f.Functions["F"].Params[0] {
					t.Fatal("Params shared with the input")
				}
			}
			if ta := cp.Types["T"]; ta != nil {
				ta.Fields["A"] = false
				if !f.Types["T"].Fields["A"] {
					t.Fatal("Fields shared with the input")
				}
			}
		})
	}
}
