package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Cross-package contracts from module-local .gon sources (v1.14 / M10).
// Executable specification for docs/rfc-source-contracts.md §7 (T1–T10).

// scModule writes a module whose .gon files each get their .go next to them
// (as `gon transpile` and the dogfood gate keep them), so that an importer
// type-checks against the package's Go form.
func scModule(t *testing.T, goVersion string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module ex\n\ngo " + goVersion + "\n"
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(rel, ".gon") && !strings.HasSuffix(rel, "_test.gon") {
			clean := preproc.Process(p, []byte(content)).Clean
			if err := os.WriteFile(strings.TrimSuffix(p, ".gon")+".go", clean, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// diagLines returns "file:line CODE" for every diagnostic line, sorted.
func diagLines(errOut string) []string {
	var out []string
	for _, l := range strings.Split(errOut, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || !strings.HasPrefix(f[2], "G") {
			continue
		}
		pos := strings.TrimSuffix(f[0], ":")
		if i := strings.LastIndex(pos, ":"); i > 0 {
			pos = pos[:i] // drop the column
		}
		out = append(out, pos+" "+strings.TrimSuffix(f[2], ":"))
	}
	sort.Strings(out)
	return out
}

func wantDiags(t *testing.T, errOut string, want ...string) {
	t.Helper()
	got := diagLines(errOut)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("diagnostics:\n got %q\nwant %q\nfull output:\n%s", got, want, errOut)
	}
}

const scLib = `package lib

type T struct{}

func Make() !*T        { return &T{} }
func Take(p !*T)       {}
func Ordinary(p *T) *T { return p }

type Box struct{ P !*T }

type Setter interface{ Set(p !*T) }

// an unexported type, reachable through an exported constructor
type hidden struct{ Field !*T }

func New() *hidden            { return &hidden{Field: &T{}} }
func (h *hidden) Get() !*T    { return h.Field }

// an unexported interface, returned by an exported function
type getter interface{ Get() !*T }

func NewGetter() getter { return New() }

// generic receivers with one and two type parameters
type G[X any] struct{}

func (g *G[X]) Put(p !*T) {}

type M[K comparable, V any] struct{}

func (m *M[K, V]) Set(k K, p !*T) {}

// a ! field promoted from an embedded struct
type Inner struct{ Q !*T }
type Outer struct {
	Inner
	R !*T
}

// an interface that embeds another
type Both interface {
	Setter
	Close() error
}
`

const scApp = `package main

import "ex/lib"

func main() {
	m := lib.Make()
	if m == nil { // 7 GW001: ! result
	}
	lib.Take(nil)                     // 9 GN001: ! parameter
	_ = lib.Box{}                     // 10 GN002: required field
	b := lib.Box{P: &lib.T{}}
	b.P = nil                         // 12 GN001: field mutation
	var s lib.Setter
	s.Set(nil)                        // 14 GN001: interface method
	x := lib.New()
	g := x.Get()
	if g == nil { // 17 GW001: exported method of an unexported type
	}
	x.Field = nil                     // 19 GN001: exported field of an unexported type
	gi := lib.NewGetter()
	v := gi.Get()
	if v == nil { // 22 GW001: exported method of an unexported interface
	}
	var gg lib.G[int]
	gg.Put(nil)                       // 25 GN001: generic receiver
	var mm lib.M[string, int]
	mm.Set("k", nil)                  // 27 GN001: two receiver type parameters
	o := lib.Outer{Inner: lib.Inner{Q: &lib.T{}}, R: &lib.T{}}
	o.Q = nil                         // 29 GN001: promoted field
	var both lib.Both
	both.Set(nil)                     // 31 GN001: method of an embedded interface
	lib.Ordinary(nil)                 // nothing, and no GW002
}
`

var scWant = []string{
	"app/main.gon:7 GW001", "app/main.gon:9 GN001", "app/main.gon:10 GN002",
	"app/main.gon:12 GN001", "app/main.gon:14 GN001", "app/main.gon:17 GW001",
	"app/main.gon:19 GN001", "app/main.gon:22 GW001", "app/main.gon:25 GN001",
	"app/main.gon:27 GN001", "app/main.gon:29 GN001", "app/main.gon:31 GN001",
}

// T1 + T3 + T4: the importer sees every kind of exported contract, including
// members of unexported types, generic receivers, promoted fields and embedded
// interfaces; no GW002 for an ordinary symbol.
func TestSourceContractsImporterSeesEveryKind(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"lib/lib.gon": scLib, "app/main.gon": scApp})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, scWant...)
}

// T10: file mode gives the importer the same diagnostics (paths differ only in
// how the file is named).
func TestSourceContractsFileModeAgrees(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"lib/lib.gon": scLib, "app/main.gon": scApp})
	chdir(t, filepath.Join(root, "app"))
	_, _, errOut := captureRun([]string{"check", "main.gon"})
	var want []string
	for _, w := range scWant {
		want = append(want, strings.TrimPrefix(w, "app/"))
	}
	wantDiags(t, errOut, want...)
}

// T3: a Go 1.27 generic method (a concrete method with its own type parameter).
func TestSourceContractsGenericMethod(t *testing.T) {
	root := scModule(t, "1.27", map[string]string{
		"lib/lib.gon":  "package lib\n\ntype Box[X any] struct{ v X }\n\nfunc (b Box[X]) Map[Y any](p !*Y) !*Y { return p }\n",
		"app/main.gon": "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tvar b lib.Box[int]\n\tb.Map((*string)(nil))\n\tr := b.Map(new(string))\n\tif r == nil {\n\t}\n}\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	// a typed nil is not a literal nil: only the result is checked here
	wantDiags(t, errOut, "app/main.gon:9 GW001")

	root2 := scModule(t, "1.27", map[string]string{
		"lib/lib.gon":  "package lib\n\ntype Box[X any] struct{ v X }\n\nfunc (b Box[X]) Map[Y any](p !*Y) !*Y { return p }\n",
		"app/main.gon": "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tvar b lib.Box[int]\n\tb.Map[string](nil)\n}\n",
	})
	chdir(t, root2)
	_, _, errOut = captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:7 GN001")
}

// T2: changing the source contract changes the importer's diagnostics.
func TestSourceContractsFollowTheSource(t *testing.T) {
	app := "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tm := lib.Make()\n\tif m == nil {\n\t}\n\tlib.Take(nil)\n}\n"
	with := scModule(t, "1.21", map[string]string{
		"lib/lib.gon":  "package lib\n\ntype T struct{}\n\nfunc Make() !*T  { return &T{} }\nfunc Take(p !*T) {}\n",
		"app/main.gon": app,
	})
	chdir(t, with)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:7 GW001", "app/main.gon:9 GN001")

	without := scModule(t, "1.21", map[string]string{
		"lib/lib.gon":  "package lib\n\ntype T struct{}\n\nfunc Make() *T  { return &T{} }\nfunc Take(p *T) {}\n",
		"app/main.gon": app,
	})
	chdir(t, without)
	_, _, errOut = captureRun([]string{"check", "./app"})
	wantDiags(t, errOut)
}

// T5: the package under check never resolves to itself, in either mode: its own
// methods (with and without `!`) produce exactly what they did before.
func TestSourceContractsNoSelfResolution(t *testing.T) {
	src := "package lib\n\ntype T struct{}\n\ntype S struct{}\n\nfunc (s *S) Set(p !*T) {}\nfunc (s *S) plain(p *T) {}\nfunc (s *S) Plain(p *T) {}\n\nfunc use(s *S) {\n\ts.Set(nil)\n\ts.plain(nil)\n\ts.Plain(nil)\n}\n"
	root := scModule(t, "1.21", map[string]string{"lib/lib.gon": src})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./lib"})
	wantDiags(t, errOut, "lib/lib.gon:12 GN001")
	chdir(t, filepath.Join(root, "lib"))
	_, _, errOut = captureRun([]string{"check", "lib.gon"})
	wantDiags(t, errOut, "lib.gon:12 GN001")
}

// T6: a .gna for the same package is ignored, whatever it says: the .gon
// contract applies in both directions, and one GW005 marks the import.
func TestSourceContractsShadowedGnaIsIgnored(t *testing.T) {
	app := "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tlib.A(nil)\n\tlib.B(nil)\n}\n"
	root := scModule(t, "1.21", map[string]string{
		// the source says: A's parameter is !, B's is not
		"lib/lib.gon": "package lib\n\nfunc A(p !*int) {}\nfunc B(p *int)  {}\n",
		// the .gna says the opposite
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\nfunctions:\n  A:\n    params: [\"*int\"]\n    results: []\n  B:\n    params: [\"!*int\"]\n    results: []\n",
		"app/main.gon":           app,
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:3 GW005", "app/main.gon:6 GN001")
	if !strings.Contains(errOut, "is ignored: the package has .gon sources") {
		t.Fatalf("GW005 message:\n%s", errOut)
	}
}

// T6: a malformed shadowed .gna is GW005, not GN003: it is never parsed.
func TestSourceContractsMalformedShadowedGna(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.gon":            "package lib\n\nfunc A(p !*int) {}\n",
		"annotations/ex/lib.gna": "not: [yaml",
		"app/main.gon":           "package main\n\nimport \"ex/lib\"\n\nfunc main() { lib.A(nil) }\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:3 GW005", "app/main.gon:5 GN001")
}

// T7: GW005 once per (file, import path): many uses in one file give one; two
// importing files give two, each at its own import; stable over repeated runs.
func TestSourceContractsGW005Unit(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/a.gon":              "package lib\n\nfunc A(p !*int) {}\n",
		"lib/b.gon":              "package lib\n\nfunc B(p !*int) {}\n",
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\n",
		"app/one.gon":            "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tx := 1\n\tlib.A(&x)\n\tlib.B(&x)\n\tlib.A(&x)\n}\n",
		"app/two.gon":            "package main\n\nimport \"ex/lib\"\n\nfunc two() { x := 1; lib.B(&x) }\n",
	})
	chdir(t, root)
	var first string
	for i := 0; i < 15; i++ {
		_, _, errOut := captureRun([]string{"check", "./app"})
		if i == 0 {
			first = errOut
			wantDiags(t, errOut, "app/one.gon:3 GW005", "app/two.gon:3 GW005")
		} else if errOut != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, errOut, first)
		}
	}
}

// T8: a main-module package with only .go and a .gna keeps using the .gna; a
// dependency outside the main module is not read even if it ships .gon.
func TestSourceContractsScope(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"plain/p.go":               "package plain\n\nfunc F(p *int) {}\n",
		"annotations/ex/plain.gna": "schema: 1\npackage: ex/plain\nfunctions:\n  F:\n    params: [\"!*int\"]\n    results: []\n",
		"app/main.gon":             "package main\n\nimport (\n\t\"ex/plain\"\n\t\"other.example/dep\"\n)\n\nfunc main() {\n\tplain.F(nil)\n\tdep.G(nil)\n}\n",
		"go.mod":                   "module ex\n\ngo 1.21\n\nrequire other.example/dep v0.0.0\n\nreplace other.example/dep => ./third_party/dep\n",
		"third_party/dep/go.mod":   "module other.example/dep\n\ngo 1.21\n",
		"third_party/dep/dep.gon":  "package dep\n\nfunc G(p !*int) {}\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	// plain: the .gna applies (GN001); dep: outside the main module, its .gon is
	// not read and it has no .gna, so nothing
	wantDiags(t, errOut, "app/main.gon:9 GN001")
}

// T9: an imported .gon that does not parse: one GN003 at the import, no
// contract applied, no fallback to a .gna.
func TestSourceContractsMalformedSource(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.go":             "package lib\n\nfunc A(p *int) {}\n",
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\nfunctions:\n  A:\n    params: [\"!*int\"]\n    results: []\n",
		"app/main.gon":           "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tlib.A(nil)\n\tlib.A(nil)\n}\n",
	})
	// a .gon that does not parse, written without its .go (the .go above is the
	// package's Go form)
	if err := os.WriteFile(filepath.Join(root, "lib", "broken.gon"), []byte("package lib\n\nfunc A( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:3 GN003")
	if !strings.Contains(errOut, "malformed source contracts for ex/lib") {
		t.Fatalf("GN003 message:\n%s", errOut)
	}
}

// T5: with no self-exclusion, a package's own derived file would shadow a
// hand-written .gna that annotates the package itself (a supported pattern):
// the .gna's `!` on S.Plain must still apply inside the package.
func TestSourceContractsOwnGnaStillAppliesInsideThePackage(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.gon":            "package lib\n\ntype S struct{}\n\nfunc (s *S) Plain(p *int) {}\n\nfunc use(s *S) {\n\ts.Plain(nil)\n}\n",
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\nmethods:\n  S.Plain:\n    params: [\"!*int\"]\n    results: []\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./lib"})
	wantDiags(t, errOut, "lib/lib.gon:8 GN001")
}

// T4: a package that mixes .gon and plain .go files: a symbol declared in the
// .go file is not derived (it can carry no `!`), and calling it is not a GW002.
func TestSourceContractsMixedPackageNoGW002(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.gon":  "package lib\n\nfunc A(p !*int) {}\n",
		"lib/extra.go": "package lib\n\nfunc Extra(p *int) {}\n",
		"app/main.gon": "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tlib.A(nil)\n\tlib.Extra(nil)\n}\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:6 GN001")
}

// A defined type over a struct (one and two hops) and an alias reach the
// importer with the base's field contracts (audit of #38).
func TestSourceContractsDefinedTypesAndAliases(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.gon": "package lib\n\ntype T struct{}\n\ntype Base struct {\n\tField !*T\n\tPlain *T\n}\n\n" +
			"type Derived Base\ntype Twice Derived\ntype Alias = Base\n\n" +
			"func NewDerived() Derived { return Derived{Field: &T{}} }\nfunc NewTwice() Twice     { return Twice{Field: &T{}} }\n",
		"app/main.gon": "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n" +
			"\tx := lib.NewDerived()\n" + //  6
			"\tx.Field = nil\n" + //           7 GN001
			"\tx.Plain = nil\n" + //           8 ok
			"\t_ = lib.Derived{}\n" + //       9 GN002
			"\ty := lib.NewTwice()\n" + //    10
			"\ty.Field = nil\n" + //          11 GN001
			"\tvar a lib.Alias\n" + //        12 GN002 (bare var, as for lib.Base)
			"\ta.Field = nil\n" + //          13 GN001
			"\t_ = lib.Alias{}\n" + //        14 GN002
			"}\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:7 GN001", "app/main.gon:9 GN002", "app/main.gon:11 GN001",
		"app/main.gon:12 GN002", "app/main.gon:13 GN001", "app/main.gon:14 GN002")
}

// The alias part was a pre-existing bug of the .gna path too (v1.13.0 reported
// none of these): an alias of an annotated external type now finds the type it
// denotes.
func TestGnaContractsReachAnAlias(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"lib/lib.go":             "package lib\n\ntype T struct{}\n\ntype Base struct{ Field *T }\n\ntype Alias = Base\n",
		"annotations/ex/lib.gna": "schema: 1\npackage: ex/lib\ntypes:\n  Base:\n    fields:\n      Field: \"!*T\"\n",
		"app/main.gon":           "package main\n\nimport \"ex/lib\"\n\nfunc main() {\n\tvar a lib.Alias\n\ta.Field = nil\n\t_ = lib.Alias{}\n}\n",
	})
	chdir(t, root)
	_, _, errOut := captureRun([]string{"check", "./app"})
	wantDiags(t, errOut, "app/main.gon:6 GN002", "app/main.gon:7 GN001", "app/main.gon:8 GN002")
}
