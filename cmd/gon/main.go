// Gon — Go with non-nil type annotations.
// Usage: gon <command> <file.gon>
// Commands: vet, transpile
package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
	"github.com/daniel-juvito/gon/transpiler"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	filename := os.Args[2]

	if !strings.HasSuffix(filename, ".gon") {
		fmt.Fprintf(os.Stderr, "gon: file must have .gon extension: %s\n", filename)
		os.Exit(1)
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		os.Exit(1)
	}

	// Pre-process: strip ! type modifiers, record positions.
	result := preproc.Process(filename, src)

	// Load .gna annotations from ./annotations relative to CWD (v1 discovery).
	reg := gna.NewRegistry()
	if err := reg.LoadDir("annotations"); err != nil {
		fmt.Fprintf(os.Stderr, "gon: annotations: %v\n", err)
		os.Exit(1)
	}
	// Also try module-relative path next to the source file.
	if abs, err := filepath.Abs(filename); err == nil {
		_ = reg.LoadDir(filepath.Join(filepath.Dir(abs), "annotations"))
		_ = reg.LoadDir(filepath.Join(filepath.Dir(abs), "..", "annotations"))
	}

	// Parse and check Gon-specific nil rules.
	c, err := checker.NewWithAnnotations(filename, result.Clean, result.NonNilOffsets, reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: parse error: %v\n", err)
		os.Exit(1)
	}

	diags := c.Check()

	hasErrors := false
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
		if d.IsError() {
			hasErrors = true
		}
	}

	if hasErrors {
		os.Exit(1)
	}

	// Validate the generated Go with the standard library type checker.
	if err := validateGo(filename, result.Clean); err != nil {
		fmt.Fprintf(os.Stderr, "gon: Go type error: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "vet":
		if len(diags) == 0 {
			fmt.Println("ok")
		}

	case "transpile":
		outFile := strings.TrimSuffix(filename, ".gon") + ".go"
		if err := transpiler.TranspileToFile(result.Clean, outFile); err != nil {
			fmt.Fprintf(os.Stderr, "gon: transpile error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("transpiled: %s\n", outFile)

	default:
		fmt.Fprintf(os.Stderr, "gon: unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: gon <command> <file.gon>")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  vet        check for nil-safety violations")
	fmt.Fprintln(os.Stderr, "  transpile  emit clean Go source (.go file)")
}

func validateGo(filename string, src []byte) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return err
	}
	conf := types.Config{Importer: importer.Default()}
	_, err = conf.Check(file.Name.Name, fset, []*ast.File{file}, nil)
	return err
}
