// Package gna loads and validates .gna nilability annotation files.
//
// A .gna file supplies nilability metadata only. go/types remains the
// authority for Go semantics. See docs/gna-spec-v1.md.
package gna

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const SupportedSchema = 1

// Signature is positional non-nil flags for parameters and results.
// true means the position carries an explicit non-nil claim ("!T").
type Signature struct {
	Params  []bool
	Results []bool
}

// File is the validated contents of one .gna file.
type File struct {
	Schema      int
	Package     string
	Description string
	Functions   map[string]*Signature // key: function name
	Methods     map[string]*Signature // key: TypeName.MethodName
	Receivers   map[string]bool       // key: TypeName; true => non-nil receiver
}

// raw YAML shapes (all type strings are quoted in the file).
type rawFile struct {
	Schema      int                       `yaml:"schema"`
	Package     string                    `yaml:"package"`
	Description string                    `yaml:"description"`
	Functions   map[string]*rawSignature  `yaml:"functions"`
	Methods     map[string]*rawSignature  `yaml:"methods"`
	Receivers   map[string]string         `yaml:"receivers"`
}

type rawSignature struct {
	Params  []string `yaml:"params"`
	Results []string `yaml:"results"`
}

// Load reads and validates a .gna file from disk.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadBytes(filepath.Base(path), data)
}

// LoadBytes parses and validates .gna content.
func LoadBytes(name string, data []byte) (*File, error) {
	var raw rawFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: yaml: %w", name, err)
	}

	if raw.Schema == 0 {
		return nil, fmt.Errorf("%s: missing required field schema", name)
	}
	if raw.Schema != SupportedSchema {
		return nil, fmt.Errorf("%s: unsupported schema version %d (supported: %d)", name, raw.Schema, SupportedSchema)
	}
	if raw.Package == "" {
		return nil, fmt.Errorf("%s: missing required field package", name)
	}

	f := &File{
		Schema:      raw.Schema,
		Package:     raw.Package,
		Description: raw.Description,
		Functions:   make(map[string]*Signature),
		Methods:     make(map[string]*Signature),
		Receivers:   make(map[string]bool),
	}

	for nameFn, rs := range raw.Functions {
		if rs == nil {
			return nil, fmt.Errorf("%s: function %q: empty signature", name, nameFn)
		}
		if _, dup := f.Functions[nameFn]; dup {
			return nil, fmt.Errorf("%s: duplicate function %q", name, nameFn)
		}
		sig, err := convertSig(name, "function "+nameFn, rs)
		if err != nil {
			return nil, err
		}
		f.Functions[nameFn] = sig
	}

	for nameM, rs := range raw.Methods {
		if rs == nil {
			return nil, fmt.Errorf("%s: method %q: empty signature", name, nameM)
		}
		if !strings.Contains(nameM, ".") {
			return nil, fmt.Errorf("%s: method key %q must be TypeName.MethodName", name, nameM)
		}
		if _, dup := f.Methods[nameM]; dup {
			return nil, fmt.Errorf("%s: duplicate method %q", name, nameM)
		}
		sig, err := convertSig(name, "method "+nameM, rs)
		if err != nil {
			return nil, err
		}
		f.Methods[nameM] = sig
	}

	for typeName, ann := range raw.Receivers {
		nn, err := parseReceiver(name, typeName, ann)
		if err != nil {
			return nil, err
		}
		f.Receivers[typeName] = nn
	}

	return f, nil
}

func convertSig(file, label string, rs *rawSignature) (*Signature, error) {
	sig := &Signature{}
	for i, p := range rs.Params {
		nn, err := parseTypeAnn(file, fmt.Sprintf("%s params[%d]", label, i), p)
		if err != nil {
			return nil, err
		}
		sig.Params = append(sig.Params, nn)
	}
	for i, r := range rs.Results {
		nn, err := parseTypeAnn(file, fmt.Sprintf("%s results[%d]", label, i), r)
		if err != nil {
			return nil, err
		}
		sig.Results = append(sig.Results, nn)
	}
	return sig, nil
}

// parseTypeAnn accepts "T" or "!T". Returns whether non-nil was claimed.
func parseTypeAnn(file, label, s string) (bool, error) {
	if s == "" {
		return false, fmt.Errorf("%s: %s: empty type annotation", file, label)
	}
	if strings.HasPrefix(s, "!") {
		if len(s) == 1 {
			return false, fmt.Errorf("%s: %s: invalid type annotation %q", file, label, s)
		}
		return true, nil
	}
	return false, nil
}

func parseReceiver(file, typeName, s string) (bool, error) {
	switch s {
	case "!":
		return true, nil
	case "":
		return false, nil
	default:
		// also accept "!T" form for consistency
		if strings.HasPrefix(s, "!") {
			return true, nil
		}
		return false, fmt.Errorf("%s: receivers.%s: expected "!" or "!T", got %q", file, typeName, s)
	}
}

// Registry holds loaded annotations keyed by package import path.
type Registry struct {
	byPackage map[string]*File
}

// NewRegistry returns an empty annotation registry.
func NewRegistry() *Registry {
	return &Registry{byPackage: make(map[string]*File)}
}

// Add registers a validated File. Duplicate package → error.
func (r *Registry) Add(f *File) error {
	if f == nil {
		return fmt.Errorf("nil File")
	}
	if _, ok := r.byPackage[f.Package]; ok {
		return fmt.Errorf("duplicate annotations for package %q", f.Package)
	}
	r.byPackage[f.Package] = f
	return nil
}

// LoadDir loads every *.gna file under dir (non-recursive).
// Packages already present in the registry are skipped (first load wins).
// Missing dir is not an error.
func (r *Registry) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gna") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := Load(path)
		if err != nil {
			return err
		}
		if r.HasPackage(f.Package) {
			continue // first load wins
		}
		if err := r.Add(f); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

// LookupFunc returns the signature for package-level function name, if any.
func (r *Registry) LookupFunc(pkg, name string) (*Signature, bool) {
	if r == nil {
		return nil, false
	}
	f, ok := r.byPackage[pkg]
	if !ok {
		return nil, false
	}
	sig, ok := f.Functions[name]
	return sig, ok
}

// LookupMethod returns the signature for TypeName.MethodName in pkg, if any.
func (r *Registry) LookupMethod(pkg, typeName, method string) (*Signature, bool) {
	if r == nil {
		return nil, false
	}
	f, ok := r.byPackage[pkg]
	if !ok {
		return nil, false
	}
	sig, ok := f.Methods[typeName+"."+method]
	return sig, ok
}

// HasPackage reports whether any annotations were loaded for pkg.
func (r *Registry) HasPackage(pkg string) bool {
	if r == nil {
		return false
	}
	_, ok := r.byPackage[pkg]
	return ok
}

// Package returns the File for pkg, if loaded.
func (r *Registry) Package(pkg string) (*File, bool) {
	if r == nil {
		return nil, false
	}
	f, ok := r.byPackage[pkg]
	return f, ok
}
