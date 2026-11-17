// Construction invariants behind the checker's own `!` contracts (dogfood D5,
// Proposal A, P1–P3). The contracts are static: Gon proves that no `nil` is
// returned, passed or stored in their place in the .gon sources. These tests
// lock the runtime side the contracts rely on: every way the values are built
// produces a non-nil value.
package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
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

// --- P2: every way a Checker is built fills fset ------------------------------

const invariantSrc = "package main\n\nfunc f() !*int {\n\treturn nil\n}\n\nfunc main() {}\n"

// newAt preprocesses invariantSrc written at dir/name and returns New's and
// NewWithAnnotations' checkers for it.
func newAt(t *testing.T, dir, name string) (plain, annotated *Checker) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(invariantSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	r := preproc.Process(path, []byte(invariantSrc))
	plain, err := New(path, r.Clean, r.NonNilOffsets)
	if err != nil {
		t.Fatal(err)
	}
	annotated, err = NewWithAnnotations(path, r.Clean, r.NonNilOffsets, nil)
	if err != nil {
		t.Fatal(err)
	}
	return plain, annotated
}

func TestNewFillsFset(t *testing.T) {
	inModule := pkgDir(t, map[string]string{})
	outside := t.TempDir() // no go.mod
	for name, dir := range map[string]string{"in a module": inModule, "outside a module": outside} {
		t.Run(name, func(t *testing.T) {
			plain, annotated := newAt(t, dir, "a.gon")
			if plain.fset == nil {
				t.Fatal("New left fset nil")
			}
			// NewWithAnnotations type-checks, which may replace fset (the
			// fallback parses again into a new FileSet).
			if annotated.fset == nil {
				t.Fatal("NewWithAnnotations left fset nil")
			}
			if annotated.info == nil {
				t.Fatal("type check did not run; the path under test was not exercised")
			}
		})
	}
}

// typeCheckFallback replaces fset with the one it parsed into.
func TestTypeCheckFallbackKeepsFset(t *testing.T) {
	plain, _ := newAt(t, t.TempDir(), "a.gon")
	plain.typeCheckFallback()
	if plain.fset == nil || plain.info == nil {
		t.Fatalf("fset=%v info=%v", plain.fset, plain.info)
	}
}

// CheckPackage builds one Checker per file with the package's FileSet. Its
// checkers are not exposed, so this locks the observable consequence: every
// diagnostic is positioned through fset (a nil fset would panic in
// Position), in each file.
func TestCheckPackageCheckersHaveFset(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": invariantSrc,
		"b.gon": "package main\n\nfunc g() !*int {\n\treturn nil\n}\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	for _, f := range res.Files {
		if len(f.Diagnostics) != 1 {
			t.Fatalf("%s: %v", f.Path, f.Diagnostics)
		}
		d := f.Diagnostics[0]
		if d.Code != "GN001" || d.Line != 4 || d.Col != 9 {
			t.Fatalf("%s: got %s at %d:%d, want GN001 at 4:9", f.Path, d.Code, d.Line, d.Col)
		}
	}
}

// --- P3: the declaration maps exist wherever a Checker is built ---------------

func TestNewFillsDeclarationMaps(t *testing.T) {
	plain, annotated := newAt(t, pkgDir(t, map[string]string{}), "a.gon")
	for name, c := range map[string]*Checker{"New": plain, "NewWithAnnotations": annotated} {
		if c.funcParams == nil || c.funcReturns == nil || c.structFields == nil {
			t.Fatalf("%s: funcParams=%v funcReturns=%v structFields=%v", name, c.funcParams, c.funcReturns, c.structFields)
		}
	}
}

// CheckPackage builds one packageShared and hands its maps to every file's
// Checker. Neither is exposed, so this locks what each map makes observable:
// a contract declared in a.gon is applied in b.gon through funcParams (a nil
// argument), funcReturns (a nil comparison of a result), structFields (a
// literal without a !-field), and b.gon's own `!` is found through offsets.
// Writing a contract into a nil map would panic in collect.
func TestCheckPackageFillsSharedDeclarationMaps(t *testing.T) {
	dir := pkgDir(t, map[string]string{
		"a.gon": "package main\n\ntype T struct{}\n\ntype Conn struct{ DB !*T }\n\nfunc Take(p !*T) {}\n\nfunc Make() !*T { return &T{} }\n\nfunc main() {}\n",
		"b.gon": "package main\n\nfunc use() {\n\tTake(nil)\n\tx := Make()\n\tif x == nil {\n\t}\n\t_ = Conn{}\n}\n\nfunc own() !*T {\n\treturn nil\n}\n",
	})
	res := mustPkg(t, dir, "a.gon", "b.gon")
	wantCodes(t, res, "b.gon", "GN001", "GN001", "GN002", "GW001")
	wantCodes(t, res, "a.gon")
}

// --- P4: every Checker holds a file -------------------------------------------
//
// A Checker is only built from a source that parsed; a source that cannot be
// obtained or parsed is rejected before any Checker exists. These tests lock
// that, and that type-checking afterwards never takes the file away.

// Source that cannot be parsed builds no Checker at all.
func TestNewWithoutParsableSourceBuildsNoChecker(t *testing.T) {
	for name, src := range map[string]string{
		"empty":             "",
		"no package clause": "func f() {}\n",
		"syntax error":      "package p\n\nfunc f( {\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := preproc.Process("x.gon", []byte(src))
			if c, err := New("x.gon", r.Clean, r.NonNilOffsets); err == nil || c != nil {
				t.Fatalf("New: checker=%v err=%v", c, err)
			}
			if c, err := NewWithAnnotations("x.gon", r.Clean, r.NonNilOffsets, nil); err == nil || c != nil {
				t.Fatalf("NewWithAnnotations: checker=%v err=%v", c, err)
			}
		})
	}
}

// New and NewWithAnnotations, in a module and outside one, hold a file, and
// so does a Checker after the fallback type check.
func TestCheckerAlwaysHasFile(t *testing.T) {
	for name, dir := range map[string]string{"in a module": pkgDir(t, map[string]string{}), "outside a module": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			plain, annotated := newAt(t, dir, "a.gon")
			if plain.file == nil {
				t.Fatal("New left file nil")
			}
			if annotated.file == nil {
				t.Fatal("NewWithAnnotations left file nil")
			}
			plain.typeCheckFallback()
			if plain.file == nil {
				t.Fatal("typeCheckFallback left file nil")
			}
			annotated.typeCheckFallback()
			if annotated.file == nil {
				t.Fatal("typeCheckFallback (after NewWithAnnotations) left file nil")
			}
		})
	}
}

// What TestTypeCheckFallbackWithoutSourceIsNoOp meant, on a Checker that can
// exist: when the fallback has no usable source to parse again (none, or one
// that does not parse), it does not panic, keeps the file the Checker holds
// (the same pointer) and type-checks that file.
func TestTypeCheckFallbackWithoutUsableSourceKeepsFile(t *testing.T) {
	for name, src := range map[string][]byte{
		"no source":         nil,
		"unparsable source": []byte("package p\n\nfunc f( {\n"),
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newAt(t, t.TempDir(), "a.gon")
			before := c.file
			c.cleanSrc = src
			c.info = nil
			c.typeCheckFallback()
			if c.file != before {
				t.Fatal("the file the Checker holds was replaced")
			}
			if c.info == nil || len(c.info.Defs) == 0 {
				t.Fatal("the file the Checker holds was not type-checked")
			}
		})
	}
}
