// Command gon is the Gon nil-safety toolchain.
//
// Exit codes:
//
//	0  success (warnings alone do not fail)
//	1  checker error, Go type error, or build failure
//	2  usage / invalid arguments / missing input
//	3  internal failure (test-only injection)
package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/daniel-juvito/gon/internal/checker"
	gonfmt "github.com/daniel-juvito/gon/internal/fmt"
	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/lsp"
	"github.com/daniel-juvito/gon/internal/preproc"
	"github.com/daniel-juvito/gon/transpiler"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the testable entry point. It returns a process exit code.
func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	cmd := args[0]
	switch cmd {
	case "help", "-h", "--help":
		usage()
		return 0
	case "version", "-version", "--version":
		fmt.Println("gon version 1.3.0")
		return 0
	}

	switch cmd {
	case "check", "vet", "transpile", "build", "fmt", "lsp":
		// ok
	default:
		fmt.Fprintf(os.Stderr, "gon: unknown command: %s\n", cmd)
		usage()
		return 2
	}

	if cmd == "lsp" {
		srv := lsp.New(os.Stdin, os.Stdout)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "gon: lsp error: %v\n", err)
			return 1
		}
		return 0
	}

	// check / vet: support --json and multiple file operands.
	if cmd == "check" || cmd == "vet" {
		return runCheck(cmd, args[1:])
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "gon: %s requires a .gon file\n", cmd)
		usage()
		return 2
	}
	filename := args[1]

	if !strings.HasSuffix(filename, ".gon") {
		fmt.Fprintf(os.Stderr, "gon: input must be a .gon file, got %s\n", filename)
		return 2
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", filename, err)
		// Non-check commands keep historical exit 1 on read failure.
		return 1
	}

	if cmd == "fmt" {
		formatted, err := gonfmt.Format(filename, src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: fmt error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(filename, formatted, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gon: write %s: %v\n", filename, err)
			return 1
		}
		return 0
	}

	result := preproc.Process(filename, src)

	startDir := filepath.Dir(filename)
	if abs, err := filepath.Abs(startDir); err == nil {
		startDir = abs
	}
	wd, err := os.Getwd()
	if err != nil {
		wd = startDir
	}
	resolver := gna.DefaultChain(wd, startDir)

	c, err := checker.NewWithAnnotations(filename, result.Clean, result.NonNilOffsets, resolver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: parse error: %v\n", err)
		return 1
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
		return 1
	}

	if err := validateGo(filename, result.Clean); err != nil {
		fmt.Fprintf(os.Stderr, "gon: Go type error: %v\n", err)
		return 1
	}

	switch cmd {
	case "transpile":
		outFile := strings.TrimSuffix(filename, ".gon") + ".go"
		if err := transpiler.TranspileToFile(result.Clean, outFile); err != nil {
			fmt.Fprintf(os.Stderr, "gon: transpile error: %v\n", err)
			return 1
		}
		fmt.Printf("transpiled: %s\n", outFile)
		return 0

	case "build":
		outFile := strings.TrimSuffix(filename, ".gon") + ".go"
		if err := transpiler.TranspileToFile(result.Clean, outFile); err != nil {
			fmt.Fprintf(os.Stderr, "gon: transpile error: %v\n", err)
			return 1
		}
		bin := strings.TrimSuffix(filepath.Base(filename), ".gon")
		goArgs := []string{"build", "-o", bin, outFile}
		if len(args) > 2 {
			goArgs = append([]string{"build"}, args[2:]...)
			goArgs = append(goArgs, outFile)
		}
		goCmd := exec.Command("go", goArgs...)
		goCmd.Stdout = os.Stdout
		goCmd.Stderr = os.Stderr
		if err := goCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "gon: go build failed: %v\n", err)
			return 1
		}
		fmt.Printf("built: %s\n", bin)
		return 0

	default:
		usage()
		return 2
	}
}

// runCheck implements check / vet, including Protocol v1 --json output.
func runCheck(cmd string, args []string) int {
	if testInjectFailure() {
		fmt.Fprintf(os.Stderr, "gon: internal failure (test injection)\n")
		return 3
	}

	jsonOut, files, err := parseCheckArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "gon: %s requires a .gon file\n", cmd)
		usage()
		return 2
	}

	source := protocolSourceFor(cmd)
	var allProtocol []protocolDiagnostic
	hasErrors := false
	anyDiag := false

	for _, filename := range files {
		if !strings.HasSuffix(filename, ".gon") {
			fmt.Fprintf(os.Stderr, "gon: input must be a .gon file, got %s\n", filename)
			return 2
		}

		fileAbs, err := protocolFilePath(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: path %s: %v\n", filename, err)
			return 2
		}

		src, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: read %s: %v\n", filename, err)
			// Protocol: missing / unreadable input → exit 2.
			return 2
		}

		result := preproc.Process(filename, src)

		startDir := filepath.Dir(filename)
		if abs, err := filepath.Abs(startDir); err == nil {
			startDir = abs
		}
		wd, err := os.Getwd()
		if err != nil {
			wd = startDir
		}
		resolver := gna.DefaultChain(wd, startDir)

		c, err := checker.NewWithAnnotations(filename, result.Clean, result.NonNilOffsets, resolver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gon: parse error: %v\n", err)
			return 1
		}

		diags := c.Check()
		for _, d := range diags {
			anyDiag = true
			if d.IsError() {
				hasErrors = true
			}
			if jsonOut {
				allProtocol = append(allProtocol, toProtocolDiagnostic(d, fileAbs, source, src))
			} else {
				// Human path: unchanged formatter.
				fmt.Fprintln(os.Stderr, d.String())
			}
		}

		if !jsonOut {
			if err := validateGo(filename, result.Clean); err != nil {
				fmt.Fprintf(os.Stderr, "gon: Go type error: %v\n", err)
				return 1
			}
		}
	}

	if jsonOut {
		if allProtocol == nil {
			allProtocol = []protocolDiagnostic{}
		}
		if err := writeProtocolJSON(allProtocol); err != nil {
			fmt.Fprintf(os.Stderr, "gon: write json: %v\n", err)
			return 3
		}
		if hasErrors {
			return 1
		}
		return 0
	}

	if hasErrors {
		return 1
	}
	if !anyDiag {
		fmt.Println("ok")
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: gon <command> [flags] <file.gon>...

Commands:
  check, vet   check for nil-safety violations (exit 1 on errors)
  fmt          format Gon source in place
  lsp          run the language server over stdio
  transpile    emit clean Go source (.go next to the input)
  build        transpile then run go build
  version      print version
  help         show this message

Flags (check/vet):
  --json       emit Diagnostic Protocol v1 JSON on stdout

Exit codes:
  0  success (warnings alone do not fail the process)
  1  checker / type / build failure
  2  usage / invalid arguments / missing input
  3  internal failure

Diagnostics (human) are printed as:
  file:line:column: severity CODE: message

See docs/diagnostic-protocol-v1.md for the JSON contract.`)
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
