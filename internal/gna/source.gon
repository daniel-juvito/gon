package gna

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Cross-package contracts from module-local .gon sources (v1.14 / M10,
// docs/rfc-source-contracts.md).
//
// For an import path inside the main module whose directory holds non-test
// .gon files, SourceResolver derives a File from those sources: the contracts
// an importer can reach (C3), keyed exactly as a hand-written .gna would key
// them, so the existing .gna machinery (resolution through go/types, GN003 /
// GW004 validation) applies unchanged. It never serves the package being
// checked (C2), and a .gna for the same path is ignored (C5).

// Locator reports whether a .gna exists for an import path, without reading it.
type Locator interface {
	Has(importPath string) bool
}

// SourceError is a module-local package's .gon source that cannot be read or
// parsed. The importer reports it once (GN003) and applies no contract (C6b).
type SourceError struct {
	ImportPath string
	Err        error
}

func (e *SourceError) Error() string {
	return "malformed source contracts for " + e.ImportPath + ": " + e.Err.Error()
}

func (e *SourceError) Unwrap() error { return e.Err }

// SourceResolver derives contracts from the .gon sources of imported packages
// of the main module (the module whose go.mod encloses StartDir).
type SourceResolver struct {
	// StartDir is the directory of the package (or file) being checked: it
	// locates the main module, and it is never served itself (C2).
	StartDir string
	// Shadowing are the .gna locations whose presence for a served path is
	// reported as GW005 (C5). They are asked whether a .gna exists, never read.
	Shadowing []Locator

	ready   bool
	root    string // main module root
	modPath string // main module path
	self    string // cleaned absolute StartDir
	cache   map[string]*File
	errs    map[string]error
}

func (s *SourceResolver) init() {
	if s.ready {
		return
	}
	s.ready = true
	s.cache = make(map[string]*File)
	s.errs = make(map[string]error)
	start := s.StartDir
	if start == "" {
		start = "."
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return
	}
	s.self = filepath.Clean(abs)
	for dir := s.self; ; {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			s.root = dir
			s.modPath = modulePathOf(data)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func modulePathOf(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`)
		}
	}
	return ""
}

// Resolve implements Resolver.
func (s *SourceResolver) Resolve(importPath string) (*File, error) {
	if s == nil || importPath == "" {
		return nil, nil
	}
	s.init()
	if s.modPath == "" {
		return nil, nil
	}
	var rel string
	switch {
	case importPath == s.modPath:
	case strings.HasPrefix(importPath, s.modPath+"/"):
		rel = strings.TrimPrefix(importPath, s.modPath+"/")
	default:
		return nil, nil // not in the main module (C1)
	}
	if strings.Contains(rel, "..") {
		return nil, nil
	}
	dir := filepath.Clean(filepath.Join(s.root, filepath.FromSlash(rel)))
	if dir == s.self {
		return nil, nil // never the package being checked (C2)
	}
	if f, ok := s.cache[importPath]; ok {
		return f, nil
	}
	if err, ok := s.errs[importPath]; ok {
		return nil, err
	}
	names, err := gonSources(dir)
	if err != nil || len(names) == 0 {
		s.cache[importPath] = nil // no .gon: the .gna resolvers apply (C6b)
		return nil, nil
	}
	f, err := deriveFile(importPath, names)
	if err != nil {
		e := &SourceError{ImportPath: importPath, Err: err}
		s.errs[importPath] = e
		return nil, e
	}
	for _, l := range s.Shadowing {
		if l != nil && l.Has(importPath) {
			f.ShadowedGNA = true
			break
		}
	}
	s.cache[importPath] = f
	return f, nil
}

// gonSources lists the non-test .gon files of dir, sorted (C7).
func gonSources(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasSuffix(n, ".gon") && !strings.HasSuffix(n, "_test.gon") {
			names = append(names, filepath.Join(dir, n))
		}
	}
	sort.Strings(names)
	return names, nil
}

// deriveFile builds the importer-reachable contract surface (C3, C4): every
// exported function, and every exported method, interface method and struct
// field whatever the name of the type that holds it. Unexported members are not
// derived.
func deriveFile(importPath string, names []string) (*File, error) {
	f := &File{
		Schema:        1,
		Package:       importPath,
		Description:   "derived from .gon sources",
		Functions:     make(map[string]*Signature),
		Methods:       make(map[string]*Signature),
		Receivers:     make(map[string]bool),
		Types:         make(map[string]*TypeAnnotation),
		SourceDerived: true,
	}
	// The exported fields of every struct type of the package, and the type
	// expression of every defined type that is not itself a struct or an
	// interface (`type Derived Base`): resolved after all files are read, since
	// the base may be declared in another file.
	structs := make(map[string]map[string]bool)
	defined := make(map[string]ast.Expr)
	for _, name := range names {
		src, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		r := preproc.Process(name, src)
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, name, r.Clean, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		nn := func(e ast.Expr) bool { return r.NonNilOffsets[fset.Position(e.Pos()).Offset] }
		flags := func(fl *ast.FieldList) []bool {
			out := []bool{}
			if fl == nil {
				return out
			}
			for _, fd := range fl.List {
				c := len(fd.Names)
				if c == 0 {
					c = 1
				}
				for i := 0; i < c; i++ {
					out = append(out, nn(fd.Type))
				}
			}
			return out
		}
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				sig := &Signature{Params: flags(d.Type.Params), Results: flags(d.Type.Results)}
				if d.Recv == nil {
					f.Functions[d.Name.Name] = sig
					continue
				}
				if t := typeNameOf(d.Recv.List[0].Type); t != "" {
					f.Methods[t+"."+d.Name.Name] = sig
				}
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts.Assign.IsValid() {
						continue // an alias declares no members of its own
					}
					switch t := ts.Type.(type) {
					case *ast.StructType:
						fields := make(map[string]bool)
						for _, fd := range t.Fields.List {
							if len(fd.Names) == 0 { // embedded: named after its type
								if n := typeNameOf(fd.Type); n != "" && ast.IsExported(n) {
									fields[n] = nn(fd.Type)
								}
								continue
							}
							for _, id := range fd.Names {
								if id.IsExported() {
									fields[id.Name] = nn(fd.Type)
								}
							}
						}
						structs[ts.Name.Name] = fields
						if len(fields) > 0 {
							f.Types[ts.Name.Name] = &TypeAnnotation{Fields: fields}
						}
					case *ast.Ident, *ast.ParenExpr, *ast.IndexExpr, *ast.IndexListExpr:
						defined[ts.Name.Name] = t
					case *ast.InterfaceType:
						for _, fd := range t.Methods.List {
							ft, ok := fd.Type.(*ast.FuncType)
							if !ok {
								continue // an embedded interface: derived where it is declared
							}
							for _, id := range fd.Names {
								if id.IsExported() {
									f.Methods[ts.Name.Name+"."+id.Name] = &Signature{Params: flags(ft.Params), Results: flags(ft.Results)}
								}
							}
						}
					}
				}
			}
		}
	}
	// A defined type whose underlying type is a struct of this package has that
	// struct's fields (Go gives it no methods of the base, so only fields are
	// copied): `type Derived Base`, `type Twice Derived`, `type D Base[int]`.
	// A base declared in another package is not followed (it has its own
	// contracts under its own name; see the RFC's implementation notes).
	for dname := range defined {
		if fields := definedStructFields(dname, defined, structs); len(fields) > 0 {
			cp := make(map[string]bool, len(fields))
			for k, v := range fields {
				cp[k] = v
			}
			f.Types[dname] = &TypeAnnotation{Fields: cp}
		}
	}
	return f, nil
}

// definedStructFields follows a chain of same-package defined types to the
// struct it ends in and returns that struct's exported field flags, or nil.
func definedStructFields(name string, defined map[string]ast.Expr, structs map[string]map[string]bool) map[string]bool {
	seen := make(map[string]bool)
	for !seen[name] {
		seen[name] = true
		if fields, ok := structs[name]; ok {
			return fields
		}
		e, ok := defined[name]
		if !ok {
			return nil
		}
		if _, isSel := e.(*ast.SelectorExpr); isSel {
			return nil
		}
		next := typeNameOf(e)
		if next == "" {
			return nil
		}
		if sel, ok := unparen(e).(*ast.IndexExpr); ok {
			if _, isSel := sel.X.(*ast.SelectorExpr); isSel {
				return nil
			}
		}
		name = next
	}
	return nil // a cycle cannot compile; nothing to derive
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// typeNameOf is the name of the type an expression denotes, without `*`,
// parentheses, a package qualifier or type arguments: Box, *Box, Box[T],
// *Box[K, V] and pkg.Box are all "Box".
func typeNameOf(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.SelectorExpr:
			return t.Sel.Name
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}
