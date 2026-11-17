package gna

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Cross-package contracts from module-local .gon sources (v1.14 / M10):
// derivation (C3, C4), main module only (C1), never itself (C2), shadowing
// (C5), a malformed source (C6b), caching (C7).

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const libSrc = `package lib

type T struct{}

// exported function: params and results
func Take(p !*T, q *T) {}
func Make() !*T { return &T{} }
func Pair() (!*T, error) { return &T{}, nil }
func Variadic(xs ...*T) {}

// exported struct, exported and unexported fields, an embedded field
type Inner struct{ Q !*T }
type Box struct {
	P  !*T
	O  *T
	p  !*T
	*Inner
}

// unexported type: exported members are reachable through an exported API
type hidden struct {
	Field !*T
	field !*T
}

func New() *hidden { return &hidden{Field: &T{}, field: &T{}} }

func (h *hidden) Get() !*T { return h.Field }
func (h *hidden) get() !*T { return h.Field }

// methods on an exported type
func (b *Box) Set(p !*T) {}
func (b Box) Peek() *T   { return nil }

// generic receivers, one and two type parameters
type G[X any] struct{}
func (g *G[X]) Put(p !*T) {}
type M[K comparable, V any] struct{}
func (m *M[K, V]) Set(k K, p !*T) {}

// interfaces: exported and unexported names, an embedded interface
type Setter interface{ Set(p !*T) }
type getter interface{ Get() !*T; get() *T }
type Both interface {
	Setter
	Close() error
}

// an alias declares nothing of its own
type Alias = Box
`

func deriveOne(t *testing.T, files map[string]string, importPath string) *File {
	t.Helper()
	root := writeTree(t, files)
	s := &SourceResolver{StartDir: filepath.Join(root, "app")}
	f, err := s.Resolve(importPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f == nil {
		t.Fatal("expected a derived file")
	}
	return f
}

func TestSourceDerivesTheImporterReachableSurface(t *testing.T) {
	f := deriveOne(t, map[string]string{
		"go.mod":       "module ex\n\ngo 1.27\n",
		"lib/lib.gon":  libSrc,
		"app/main.gon": "package main\n",
	}, "ex/lib")

	if !f.SourceDerived || f.Package != "ex/lib" || f.Schema != 1 {
		t.Fatalf("header: %+v", f)
	}
	wantFuncs := map[string]*Signature{
		"Take":     {Params: []bool{true, false}, Results: []bool{}},
		"Make":     {Params: []bool{}, Results: []bool{true}},
		"Pair":     {Params: []bool{}, Results: []bool{true, false}},
		"Variadic": {Params: []bool{false}, Results: []bool{}},
		"New":      {Params: []bool{}, Results: []bool{false}},
	}
	if !reflect.DeepEqual(f.Functions, wantFuncs) {
		t.Fatalf("functions:\n got %v\nwant %v", dump(f.Functions), dump(wantFuncs))
	}
	wantMethods := map[string]*Signature{
		"hidden.Get": {Params: []bool{}, Results: []bool{true}}, // unexported type, exported method
		"Box.Set":    {Params: []bool{true}, Results: []bool{}},
		"Box.Peek":   {Params: []bool{}, Results: []bool{false}},
		"G.Put":      {Params: []bool{true}, Results: []bool{}}, // generic receiver, key without type args
		"M.Set":      {Params: []bool{false, true}, Results: []bool{}},
		"Setter.Set": {Params: []bool{true}, Results: []bool{}},
		"getter.Get": {Params: []bool{}, Results: []bool{true}}, // unexported interface, exported method
		"Both.Close": {Params: []bool{}, Results: []bool{false}},
	}
	if !reflect.DeepEqual(f.Methods, wantMethods) {
		t.Fatalf("methods:\n got %v\nwant %v", dump(f.Methods), dump(wantMethods))
	}
	wantTypes := map[string]*TypeAnnotation{
		"Inner":  {Fields: map[string]bool{"Q": true}},
		"Box":    {Fields: map[string]bool{"P": true, "O": false, "Inner": false}}, // embedded *Inner named Inner
		"hidden": {Fields: map[string]bool{"Field": true}},                         // unexported struct, exported field
	}
	if !reflect.DeepEqual(f.Types, wantTypes) {
		t.Fatalf("types:\n got %v\nwant %v", dumpT(f.Types), dumpT(wantTypes))
	}
	if len(f.Receivers) != 0 {
		t.Fatal("receiver contracts are out of scope")
	}
}

// A Go 1.27 generic method (a concrete method with its own type parameters) is
// keyed like any method: its type parameters do not change the key.
func TestSourceGenericMethodKey(t *testing.T) {
	f := deriveOne(t, map[string]string{
		"go.mod":       "module ex\n\ngo 1.27\n",
		"lib/lib.gon":  "package lib\n\ntype Box[X any] struct{ v X }\n\nfunc (b Box[X]) Map[Y any](p !*Y) !*Y { return p }\n",
		"app/main.gon": "package main\n",
	}, "ex/lib")
	want := &Signature{Params: []bool{true}, Results: []bool{true}}
	if !reflect.DeepEqual(f.Methods["Box.Map"], want) || len(f.Methods) != 1 {
		t.Fatalf("methods: %v", dump(f.Methods))
	}
}

// Gon has no syntax for a `!` on an embedded field (`struct{ !*Inner }` does not
// parse, measured on v1.13.0), so the embedded case is a `!` field PROMOTED from
// an embedded struct: it is derived on the struct that declares it, and the
// embedded field itself is derived (ordinary) under its type name.
func TestSourceEmbeddedAndPromotedFields(t *testing.T) {
	f := deriveOne(t, map[string]string{
		"go.mod":       "module ex\n\ngo 1.21\n",
		"lib/lib.gon":  "package lib\n\ntype T struct{}\n\ntype Inner struct{ Q !*T }\n\ntype Outer struct {\n\tInner\n\tR !*T\n}\n",
		"app/main.gon": "package main\n",
	}, "ex/lib")
	want := map[string]*TypeAnnotation{
		"Inner": {Fields: map[string]bool{"Q": true}},
		"Outer": {Fields: map[string]bool{"Inner": false, "R": true}},
	}
	if !reflect.DeepEqual(f.Types, want) {
		t.Fatalf("types: %v", dumpT(f.Types))
	}
}

// Test files and unexported-only declarations are not derived; the files are
// read in sorted order, so two runs give the same file.
func TestSourceIgnoresTestFilesAndIsDeterministic(t *testing.T) {
	files := map[string]string{
		"go.mod":         "module ex\n\ngo 1.21\n",
		"lib/b.gon":      "package lib\n\nfunc B(p !*int) {}\n",
		"lib/a.gon":      "package lib\n\nfunc A() !*int { x := 1; return &x }\n",
		"lib/a_test.gon": "package lib\n\nfunc FromTest(p !*int) {}\n",
		"app/main.gon":   "package main\n",
	}
	f1 := deriveOne(t, files, "ex/lib")
	f2 := deriveOne(t, files, "ex/lib")
	if _, ok := f1.Functions["FromTest"]; ok {
		t.Fatal("a *_test.gon file must not be derived")
	}
	if len(f1.Functions) != 2 || !reflect.DeepEqual(f1.Functions, f2.Functions) {
		t.Fatalf("functions: %v / %v", dump(f1.Functions), dump(f2.Functions))
	}
}

// C1 and C2: only imports inside the main module, never the directory being
// checked, and a module-local package without .gon is left to the .gna chain.
func TestSourceScope(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.mod":       "module ex\n\ngo 1.21\n",
		"lib/lib.gon":  "package lib\n\nfunc F(p !*int) {}\n",
		"plain/p.go":   "package plain\n\nfunc F(p *int) {}\n",
		"app/main.gon": "package main\n",
	})
	s := &SourceResolver{StartDir: filepath.Join(root, "app")}
	for _, path := range []string{"fmt", "example.com/other/lib", "ex/plain", "ex/app", "ex/../ex/lib"} {
		if f, err := s.Resolve(path); f != nil || err != nil {
			t.Errorf("%s must not be served: %v %v", path, f, err)
		}
	}
	if f, err := s.Resolve("ex/lib"); f == nil || err != nil {
		t.Fatalf("ex/lib must be served: %v", err)
	}
	// the package being checked is never served, even when it has .gon
	self := &SourceResolver{StartDir: filepath.Join(root, "lib")}
	if f, _ := self.Resolve("ex/lib"); f != nil {
		t.Fatal("the package under check must not resolve to itself (C2)")
	}
}

// C5: a .gna in either location is only located (Has), never read; the derived
// file records that it shadows one.
func TestSourceShadowingAndDefaultChain(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.mod":                 "module ex\n\ngo 1.21\n",
		"lib/lib.gon":            "package lib\n\nfunc F(p !*int) {}\n",
		"annotations/ex/lib.gna": "this is not YAML at all: [",
		"app/main.gon":           "package main\n",
	})
	c := DefaultChain(root, filepath.Join(root, "app"))
	f, err := c.Resolve("ex/lib")
	if err != nil {
		t.Fatalf("a shadowed .gna must not be parsed: %v", err)
	}
	if f == nil || !f.SourceDerived || !f.ShadowedGNA {
		t.Fatalf("derived file: %+v", f)
	}
	if !reflect.DeepEqual(f.Functions["F"], &Signature{Params: []bool{true}, Results: []bool{}}) {
		t.Fatalf("the .gon contract applies: %v", dump(f.Functions))
	}
	if !(&DirResolver{Root: filepath.Join(root, "annotations")}).Has("ex/lib") ||
		(&DirResolver{Root: filepath.Join(root, "annotations")}).Has("ex/none") {
		t.Fatal("DirResolver.Has")
	}
	if !(&ModuleResolver{StartDir: filepath.Join(root, "app")}).Has("ex/lib") {
		t.Fatal("ModuleResolver.Has")
	}
}

// C6b: a .gon that does not parse is a SourceError, cached, with no fallback.
func TestSourceMalformedIsAnError(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.mod":                 "module ex\n\ngo 1.21\n",
		"lib/lib.gon":            "package lib\n\nfunc F( {\n",
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\nfunctions:\n  F:\n    params: [\"!*int\"]\n    results: []\n",
		"app/main.gon":           "package main\n",
	})
	c := DefaultChain(root, filepath.Join(root, "app"))
	f, err := c.Resolve("ex/lib")
	var se *SourceError
	if f != nil || !errors.As(err, &se) || se.ImportPath != "ex/lib" {
		t.Fatalf("want a SourceError and no file (no fallback to the .gna): %v %v", f, err)
	}
	if _, err2 := c.Resolve("ex/lib"); !errors.As(err2, &se) {
		t.Fatal("the error is cached")
	}
}

func dump(m map[string]*Signature) map[string]Signature {
	out := map[string]Signature{}
	for k, v := range m {
		out[k] = *v
	}
	return out
}

func dumpT(m map[string]*TypeAnnotation) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for k, v := range m {
		out[k] = v.Fields
	}
	return out
}
