package checker

import (
	"path/filepath"
	"testing"

	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// External-package proof: non-stdlib import path resolved generically via
// go/packages identity + Resolver + .gna contracts.

func TestExternalPackageFunctionGN001(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.Load(filepath.Join("..", "..", "annotations", "github.com", "daniel-juvito", "gon", "internal", "extlib.gna"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("expected GN001 for extlib.Take(nil), got %v", diags)
	}
}

func TestExternalPackageMethodGN001(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.Load(filepath.Join("..", "..", "annotations", "github.com", "daniel-juvito", "gon", "internal", "extlib.gna"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f(h *extlib.Handle) {
	h.Put(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("expected GN001 for Handle.Put(nil), got %v", diags)
	}
}

func TestExternalPackageOrdinaryAllowsNil(t *testing.T) {
	reg := gna.NewRegistry()
	f, err := gna.Load(filepath.Join("..", "..", "annotations", "github.com", "daniel-juvito", "gon", "internal", "extlib.gna"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(f); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	_ = extlib.Echo(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	for _, d := range diags {
		if d.Code == "GN001" {
			t.Fatalf("Echo is ordinary; must allow nil: %v", diags)
		}
	}
}

func TestExternalPackageViaDirResolver(t *testing.T) {
	annRoot := filepath.Join("..", "..", "annotations")
	resolver := &gna.DirResolver{Root: annRoot}
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, resolver)
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("DirResolver path: expected GN001, got %v", diags)
	}
}

func TestExternalPackageUnannotatedIsOrdinary(t *testing.T) {
	src := `package main
import "github.com/daniel-juvito/gon/internal/extlib"
func f() {
	extlib.Take(nil)
}
`
	result := preproc.Process("test.gon", []byte(src))
	c, err := NewWithAnnotations("test.gon", result.Clean, result.NonNilOffsets, gna.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	diags := c.Check()
	if len(diags) != 0 {
		t.Fatalf("unannotated external package must be ordinary: %v", diags)
	}
}
