package gna

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver finds nilability annotations for a Go import path.
//
// Contract — three distinct outcomes, never collapsed:
//
//	found + valid     → (*File, nil)
//	not found         → (nil, nil)   // ordinary fallback; not an error
//	found + malformed → (nil, error) // hard error; must not fall through
type Resolver interface {
	Resolve(importPath string) (*File, error)
}

// Chain tries resolvers in order. First found+valid wins.
// A malformed result stops the chain immediately (no fallback).
type Chain struct {
	Resolvers []Resolver
}

// Resolve implements Resolver.
func (c *Chain) Resolve(importPath string) (*File, error) {
	if c == nil {
		return nil, nil
	}
	for _, r := range c.Resolvers {
		if r == nil {
			continue
		}
		f, err := r.Resolve(importPath)
		if err != nil {
			return nil, err
		}
		if f != nil {
			return f, nil
		}
	}
	return nil, nil
}

// DirResolver looks up annotations/<importPath>.gna under a fixed root.
// For import path "io" → <root>/io.gna
// For "github.com/foo/bar" → <root>/github.com/foo/bar.gna
type DirResolver struct {
	Root string // annotations directory
}

// Resolve implements Resolver.
func (d *DirResolver) Resolve(importPath string) (*File, error) {
	if d == nil || d.Root == "" || importPath == "" {
		return nil, nil
	}
	// Prevent path traversal via import path.
	if strings.Contains(importPath, "..") {
		return nil, nil
	}
	path := filepath.Join(d.Root, importPath+".gna")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	f, err := LoadBytes(filepath.Base(path), data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// Package identity in the file must match the import path we resolved.
	if f.Package != importPath {
		return nil, fmt.Errorf("%s: package field %q does not match import path %q", path, f.Package, importPath)
	}
	return f, nil
}

// ModuleResolver resolves against <moduleRoot>/annotations/<importPath>.gna.
// Module root is discovered by walking up from StartDir looking for go.mod.
type ModuleResolver struct {
	StartDir string
	// moduleRoot is cached after first successful discovery; empty means unknown.
	moduleRoot string
	lookedUp   bool
}

// Resolve implements Resolver.
func (m *ModuleResolver) Resolve(importPath string) (*File, error) {
	if m == nil || importPath == "" {
		return nil, nil
	}
	root, err := m.findModuleRoot()
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, nil
	}
	dir := &DirResolver{Root: filepath.Join(root, "annotations")}
	return dir.Resolve(importPath)
}

func (m *ModuleResolver) findModuleRoot() (string, error) {
	if m.lookedUp {
		return m.moduleRoot, nil
	}
	m.lookedUp = true
	dir := m.StartDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		if st, err := os.Stat(gomod); err == nil && !st.IsDir() {
			m.moduleRoot = dir
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// filesystem root; no go.mod
			return "", nil
		}
		dir = parent
	}
}

// Registry resolves from packages already loaded into memory.
// Useful for tests and as a pre-populated cache.
func (r *Registry) Resolve(importPath string) (*File, error) {
	if r == nil {
		return nil, nil
	}
	f, ok := r.byPackage[importPath]
	if !ok {
		return nil, nil
	}
	return f, nil
}

// DefaultChain builds the v1 resolver chain for CLI use:
//  1. ./annotations relative to wd
//  2. module-root/annotations (via go.mod discovery from startDir)
func DefaultChain(wd, startDir string) *Chain {
	return &Chain{Resolvers: []Resolver{
		&DirResolver{Root: filepath.Join(wd, "annotations")},
		&ModuleResolver{StartDir: startDir},
	}}
}
