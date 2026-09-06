package gna

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIOAnnotation(t *testing.T) {
	// Load the real reference file from the module.
	path := filepath.Join("..", "..", "annotations", "io.gna")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load io.gna: %v", err)
	}
	if f.Schema != 1 || f.Package != "io" {
		t.Fatalf("unexpected header: schema=%d package=%q", f.Schema, f.Package)
	}
	sig, ok := f.Methods["Writer.Write"]
	if !ok {
		t.Fatal("missing Writer.Write")
	}
	if len(sig.Params) != 1 || sig.Params[0] {
		t.Fatalf("Writer.Write params should be ordinary []byte, got %v", sig.Params)
	}
	if len(sig.Results) != 2 || sig.Results[0] || sig.Results[1] {
		t.Fatalf("Writer.Write results should be ordinary, got %v", sig.Results)
	}
}

func TestNonNilParamParsed(t *testing.T) {
	src := `
schema: 1
package: example
functions:
  Take:
    params:
      - "!string"
      - "int"
    results:
      - "!example.T"
      - "error"
`
	f, err := LoadBytes("example.gna", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	sig := f.Functions["Take"]
	if !sig.Params[0] || sig.Params[1] {
		t.Fatalf("params: %v", sig.Params)
	}
	if !sig.Results[0] || sig.Results[1] {
		t.Fatalf("results: %v", sig.Results)
	}
}

// v1.6 / M1b (rfc-type-coverage.md C10): a leading "!" binds only the
// outermost type constructor. An "!" in element position is reserved for M2b
// and must be rejected, not silently ignored.
func TestOutermostBangOnReferenceKindsParsed(t *testing.T) {
	src := `
schema: 1
package: example
functions:
  Build:
    params:
      - "![]byte"
      - "!map[string]*T"
    results:
      - "!func() error"
      - "!chan int"
`
	f, err := LoadBytes("example.gna", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	sig := f.Functions["Build"]
	if !sig.Params[0] || !sig.Params[1] || !sig.Results[0] || !sig.Results[1] {
		t.Fatalf("outermost ! on slice/map/func/chan must all parse non-nil: %v %v", sig.Params, sig.Results)
	}
}

// v1.7 / M2b (rfc-element-contract-construction.md E8): element-position "!"
// is legal in .gna type strings for slices, arrays, and map values. The
// returned flag still reflects only the outermost "!".
func TestElementPositionBangAccepted(t *testing.T) {
	src := `
schema: 1
package: example
functions:
  Build:
    params:
      - "map[string]!*Service"
      - "[8]!*Slot"
    results:
      - "[]!*T"
      - "![]!*T"
`
	f, err := LoadBytes("example.gna", []byte(src))
	if err != nil {
		t.Fatalf("element-position ! must load: %v", err)
	}
	sig := f.Functions["Build"]
	if sig.Params[0] || sig.Params[1] || sig.Results[0] {
		t.Fatalf("element-only ! must not set the outermost flag: %v %v", sig.Params, sig.Results)
	}
	if !sig.Results[1] {
		t.Fatalf("leading ! in \"![]!*T\" must set the outermost flag: %v", sig.Results)
	}
}

// E11 / O1: an "!" on a channel element or a map key remains a load error.
func TestChannelElementBangRejected(t *testing.T) {
	for _, ann := range []string{"chan !*T", "chan<- !T", "<-chan !*T", "map[!*K]V"} {
		src := "schema: 1\npackage: example\nfunctions:\n  Build:\n    results:\n      - \"" + ann + "\"\n"
		if _, err := LoadBytes("example.gna", []byte(src)); err == nil {
			t.Fatalf("channel element ! (%q) must be rejected", ann)
		}
	}
}

func TestUnsupportedSchema(t *testing.T) {
	_, err := LoadBytes("x.gna", []byte("schema: 99\npackage: x\n"))
	if err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("expected unsupported schema error, got %v", err)
	}
}

func TestMissingSchema(t *testing.T) {
	_, err := LoadBytes("x.gna", []byte("package: x\n"))
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("expected missing schema error, got %v", err)
	}
}

func TestMissingPackage(t *testing.T) {
	_, err := LoadBytes("x.gna", []byte("schema: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "package") {
		t.Fatalf("expected missing package error, got %v", err)
	}
}

func TestMethodKeyMustContainDot(t *testing.T) {
	src := `
schema: 1
package: x
methods:
  Write:
    params:
      - "[]byte"
`
	_, err := LoadBytes("x.gna", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "TypeName.MethodName") {
		t.Fatalf("expected method key error, got %v", err)
	}
}

func TestMalformedYAML(t *testing.T) {
	_, err := LoadBytes("x.gna", []byte("schema: [\n"))
	if err == nil {
		t.Fatal("expected yaml error")
	}
}

func TestReceivers(t *testing.T) {
	src := `
schema: 1
package: x
receivers:
  Writer: "!"
`
	f, err := LoadBytes("x.gna", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Receivers["Writer"] {
		t.Fatal("expected Writer receiver non-nil")
	}
}

func TestRegistryLoadDirAndLookup(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
schema: 1
package: demo
functions:
  F:
    params:
      - "!int"
    results:
      - "error"
methods:
  T.M:
    params:
      - "string"
    results:
      - "!demo.T"
`)
	if err := os.WriteFile(filepath.Join(dir, "demo.gna"), content, 0644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if !r.HasPackage("demo") {
		t.Fatal("expected demo package")
	}
	sig, ok := r.LookupFunc("demo", "F")
	if !ok || !sig.Params[0] {
		t.Fatalf("LookupFunc: ok=%v sig=%v", ok, sig)
	}
	// Missing annotation → ordinary (not found)
	if _, ok := r.LookupFunc("demo", "Missing"); ok {
		t.Fatal("missing function should not be found")
	}
	if _, ok := r.LookupFunc("other", "F"); ok {
		t.Fatal("other package should not be found")
	}
	msig, ok := r.LookupMethod("demo", "T", "M")
	if !ok || msig.Params[0] || !msig.Results[0] {
		t.Fatalf("LookupMethod: ok=%v sig=%+v", ok, msig)
	}
}

func TestRegistryMissingDirIsOK(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDir(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("missing dir should be ok: %v", err)
	}
}

func TestDuplicatePackage(t *testing.T) {
	r := NewRegistry()
	f1, err := LoadBytes("a.gna", []byte("schema: 1\npackage: p\n"))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := LoadBytes("b.gna", []byte("schema: 1\npackage: p\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Add(f1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(f2); err == nil {
		t.Fatal("expected duplicate package error")
	}
}

func TestLoadOSAnnotation(t *testing.T) {
	path := filepath.Join("..", "..", "annotations", "os.gna")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load os.gna: %v", err)
	}
	if f.Schema != 1 || f.Package != "os" {
		t.Fatalf("unexpected header: schema=%d package=%q", f.Schema, f.Package)
	}
	sig, ok := f.Methods["File.Write"]
	if !ok {
		t.Fatal("missing File.Write")
	}
	if len(sig.Params) != 1 || sig.Params[0] {
		t.Fatalf("File.Write params should be ordinary []byte, got %v", sig.Params)
	}
}
