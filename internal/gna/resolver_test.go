package gna

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirResolverFound(t *testing.T) {
	root := t.TempDir()
	content := []byte(`
schema: 1
package: io
methods:
  Writer.Write:
    params:
      - "[]byte"
`)
	if err := os.WriteFile(filepath.Join(root, "io.gna"), content, 0644); err != nil {
		t.Fatal(err)
	}
	r := &DirResolver{Root: root}
	f, err := r.Resolve("io")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Package != "io" {
		t.Fatalf("expected io package, got %+v", f)
	}
	if _, ok := f.Methods["Writer.Write"]; !ok {
		t.Fatal("missing Writer.Write")
	}
}

func TestDirResolverNotFound(t *testing.T) {
	r := &DirResolver{Root: t.TempDir()}
	f, err := r.Resolve("missing")
	if err != nil {
		t.Fatal(err)
	}
	if f != nil {
		t.Fatalf("expected nil (not found), got %+v", f)
	}
}

func TestDirResolverMalformed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.gna"), []byte("schema: 99\npackage: bad\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r := &DirResolver{Root: root}
	f, err := r.Resolve("bad")
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if f != nil {
		t.Fatal("expected nil file on error")
	}
	if !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDirResolverPackageMismatch(t *testing.T) {
	root := t.TempDir()
	// File named io.gna but package field says "os"
	if err := os.WriteFile(filepath.Join(root, "io.gna"), []byte("schema: 1\npackage: os\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r := &DirResolver{Root: root}
	_, err := r.Resolve("io")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected package mismatch error, got %v", err)
	}
}

func TestDirResolverNestedImportPath(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("github.com", "foo", "bar.gna")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("schema: 1\npackage: github.com/foo/bar\n")
	if err := os.WriteFile(filepath.Join(root, rel), content, 0644); err != nil {
		t.Fatal(err)
	}
	r := &DirResolver{Root: root}
	f, err := r.Resolve("github.com/foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Package != "github.com/foo/bar" {
		t.Fatalf("got %+v", f)
	}
}

func TestChainFallback(t *testing.T) {
	// First resolver empty; second has the package.
	empty := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(second, "demo.gna"), []byte("schema: 1\npackage: demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	chain := &Chain{Resolvers: []Resolver{
		&DirResolver{Root: empty},
		&DirResolver{Root: second},
	}}
	f, err := chain.Resolve("demo")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Package != "demo" {
		t.Fatalf("expected fallback to second resolver, got %+v", f)
	}
}

func TestChainMalformedNoFallback(t *testing.T) {
	// First resolver finds a broken file — must not fall through to second.
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "demo.gna"), []byte("schema: 99\npackage: demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "demo.gna"), []byte("schema: 1\npackage: demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	chain := &Chain{Resolvers: []Resolver{
		&DirResolver{Root: first},
		&DirResolver{Root: second},
	}}
	f, err := chain.Resolve("demo")
	if err == nil {
		t.Fatal("expected hard error, no fallback")
	}
	if f != nil {
		t.Fatal("expected nil file")
	}
}

func TestChainFirstWins(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "demo.gna"), []byte("schema: 1\npackage: demo\ndescription: first\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "demo.gna"), []byte("schema: 1\npackage: demo\ndescription: second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	chain := &Chain{Resolvers: []Resolver{
		&DirResolver{Root: first},
		&DirResolver{Root: second},
	}}
	f, err := chain.Resolve("demo")
	if err != nil {
		t.Fatal(err)
	}
	if f.Description != "first" {
		t.Fatalf("expected first wins, got %q", f.Description)
	}
}

func TestModuleResolver(t *testing.T) {
	// Fake module layout: <root>/go.mod + <root>/annotations/io.gna
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/mod\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ann := filepath.Join(root, "annotations")
	if err := os.MkdirAll(ann, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ann, "io.gna"), []byte("schema: 1\npackage: io\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Start from a nested dir to force walk-up.
	start := filepath.Join(root, "cmd", "tool")
	if err := os.MkdirAll(start, 0755); err != nil {
		t.Fatal(err)
	}
	m := &ModuleResolver{StartDir: start}
	f, err := m.Resolve("io")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Package != "io" {
		t.Fatalf("expected io via module resolver, got %+v", f)
	}
}

func TestModuleResolverNoGoMod(t *testing.T) {
	// Temp dir without go.mod → not found, not error.
	m := &ModuleResolver{StartDir: t.TempDir()}
	f, err := m.Resolve("io")
	if err != nil {
		t.Fatal(err)
	}
	if f != nil {
		t.Fatalf("expected nil, got %+v", f)
	}
}

func TestRegistryImplementsResolver(t *testing.T) {
	reg := NewRegistry()
	f, err := LoadBytes("demo.gna", []byte("schema: 1\npackage: demo\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	var r Resolver = reg
	got, err := r.Resolve("demo")
	if err != nil || got == nil || got.Package != "demo" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	miss, err := r.Resolve("other")
	if err != nil || miss != nil {
		t.Fatalf("expected miss, got %+v err=%v", miss, err)
	}
}

func TestDefaultChainResolvesRepoAnnotations(t *testing.T) {
	// Resolve real annotations/io.gna via module-relative path from this package dir.
	// Test runs with cwd = package dir (internal/gna); module root is ../..
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Prefer DirResolver pointed at ../../annotations when present.
	annRoot := filepath.Join(wd, "..", "..", "annotations")
	if _, err := os.Stat(filepath.Join(annRoot, "io.gna")); err != nil {
		t.Skip("repo annotations not available")
	}
	chain := &Chain{Resolvers: []Resolver{
		&DirResolver{Root: annRoot},
	}}
	f, err := chain.Resolve("io")
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.Package != "io" {
		t.Fatalf("expected io, got %+v", f)
	}
	f2, err := chain.Resolve("os")
	if err != nil {
		t.Fatal(err)
	}
	if f2 == nil || f2.Package != "os" {
		t.Fatalf("expected os, got %+v", f2)
	}
}
