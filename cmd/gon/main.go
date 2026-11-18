// Command gon is the Gon nil-safety toolchain.
//
// Exit codes:
//
//	0  success (warnings alone do not fail)
//	1  checker error, Go type error, or build failure
//	2  usage / invalid arguments / missing input / a package or module that
//	   cannot be loaded
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
	"github.com/daniel-juvito/gon/internal/goenv"
	"github.com/daniel-juvito/gon/internal/lsp"
	"github.com/daniel-juvito/gon/internal/preproc"
	"github.com/daniel-juvito/gon/transpiler"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/tools/go/packages"
)

func main() {
	// Release binaries are built with -trimpath, which empties go/build's
	// GOROOT; without it a file outside any module cannot import the stdlib.
	goenv.EnsureGOROOT()
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
		fmt.Println("gon version 1.17.0")
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

	// v1.12 (M9): a directory, or `dir/...`, selects package mode for the
	// commands that emit code. A file operand keeps the single-file path below.
	if (cmd == "transpile" || cmd == "build") && isDirOperand(filename) {
		return runEmitDir(cmd, args[1:])
	}

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

	goCtx, err := loadGoContext(filename, result.Clean)
	if err != nil {
		// The module the file is in cannot be loaded (v1.17): nothing is
		// checked or written.
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return 2
	}

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

	if err := goCtx.validate(filename, result.Clean); err != nil {
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
	typeErr := false

	targets, err := expandCheckOperands(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gon: %v\n", err)
		return 2
	}

	checkedDirs := make(map[string]bool)
	for _, t := range targets {
		if t.dir {
			// Package mode: the directory's .gon files are checked together.
			out, skipped, code := checkPackageDir(t, jsonOut, source)
			if code != 0 {
				return code
			}
			if skipped {
				continue
			}
			checkedDirs[t.path] = true
			hasErrors = hasErrors || out.hasErrors
			anyDiag = anyDiag || out.anyDiag
			allProtocol = append(allProtocol, out.protocol...)
			for _, te := range out.typeErrors {
				fmt.Fprintf(os.Stderr, "gon: Go type error: %s\n", te)
				typeErr = true
			}
			continue
		}
		filename := t.path
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

		goCtx, err := loadGoContext(filename, result.Clean)
		if err != nil {
			// The module the file is in cannot be loaded (v1.17): exit 2
			// before emission, so no envelope (Protocol v1 §7).
			fmt.Fprintf(os.Stderr, "gon: %v\n", err)
			return 2
		}

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

		if err := goCtx.validate(filename, result.Clean); err != nil {
			fmt.Fprintf(os.Stderr, "gon: Go type error: %v\n", err)
			if !jsonOut {
				return 1
			}
			// --json: the document must still be written, and it holds only
			// Protocol v1 diagnostics. The type error goes to stderr and decides
			// the exit code (Protocol v1 §7).
			typeErr = true
		}
	}

	warnEmptyPatterns(targets, checkedDirs)

	if jsonOut {
		if allProtocol == nil {
			allProtocol = []protocolDiagnostic{}
		}
		if err := writeProtocolJSON(allProtocol); err != nil {
			fmt.Fprintf(os.Stderr, "gon: write json: %v\n", err)
			return 3
		}
		if hasErrors || typeErr {
			return 1
		}
		return 0
	}

	if hasErrors || typeErr {
		return 1
	}
	if !anyDiag {
		fmt.Println("ok")
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: gon <command> [flags] <file.gon | dir>...

Commands:
  check, vet   check for nil-safety violations (exit 1 on errors); a file is
               checked alone, a directory (or dir/...) as one package
  fmt          format Gon source in place
  lsp          run the language server over stdio
  transpile    emit clean Go source (.go next to the input); a directory
               (or dir/...) is checked as one package and written only if it
               passes
  build        transpile then run go build; a directory builds the package
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

// goContext is the Go context a .gon file is checked in (file mode). A file
// inside a module is type-checked against the real module graph, loaded once
// by loadGoContext before the file is checked; a file outside any module is
// type-checked with the GOROOT/GOPATH importer.
type goContext struct {
	inModule bool
	typeErr  error // the module load's parse or type error for the file
}

// loadGoContext loads the module that contains filename, with src (the file's
// clean Go form) overlaid as a package of it. Since v1.17 a go.mod that is
// found but whose module cannot be loaded is an error, not a fall back to the
// GOROOT/GOPATH importer:
//
//   - no go.mod above the file (K0): no error; validate uses the importer.
//   - the load fails and golang.org/x/mod shows that go.mod is invalid (it does
//     not parse, has no module directive, or its module path is not a valid
//     import path) (K1): "invalid go.mod: ...".
//   - the load fails for any other reason (K2): the go command's error as it
//     is, without a guessed cause.
//   - the load succeeds (K3): validate reports its parse or type error.
//
// x/mod only explains a load that already failed: a go.mod the go command
// accepts is never rejected.
func loadGoContext(filename string, src []byte) (goContext, error) {
	start := filepath.Dir(filename)
	if abs, aerr := filepath.Abs(start); aerr == nil {
		start = abs
	}
	root := findGoModRoot(start)
	if root == "" {
		return goContext{}, nil
	}

	overlayPath := filepath.Join(root, "__gon_validate__", "check.go")
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedImports | packages.NeedDeps,
		Dir:     root,
		Overlay: map[string][]byte{overlayPath: src},
		Tests:   false,
	}
	pkgs, lerr := packages.Load(cfg, "file="+overlayPath)
	if lerr != nil {
		if reason := invalidGoMod(filepath.Join(root, "go.mod")); reason != "" {
			return goContext{}, fmt.Errorf("invalid go.mod: %s", reason)
		}
		return goContext{}, fmt.Errorf("cannot load the module of %s: %v", filename, lerr)
	}
	if len(pkgs) == 0 {
		// Not observed; kept as before v1.17 (the importer decides).
		return goContext{}, nil
	}
	ctx := goContext{inModule: true}
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			if e.Kind == packages.TypeError || e.Kind == packages.ParseError {
				ctx.typeErr = fmt.Errorf("%s", e)
				return ctx, nil
			}
		}
	}
	return ctx, nil
}

// invalidGoMod returns why the go.mod at path is invalid, as golang.org/x/mod
// shows it, or "" when x/mod finds nothing wrong (or it cannot be read).
func invalidGoMod(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return err.Error()
	}
	if f.Module == nil {
		return path + ": no module directive"
	}
	if err := module.CheckImportPath(f.Module.Mod.Path); err != nil {
		return path + ": " + err.Error()
	}
	return ""
}

// validate type-checks the clean Go form of filename in its context: the
// module load's result for a file inside a module, the GOROOT/GOPATH importer
// for a file outside any module.
func (g goContext) validate(filename string, src []byte) error {
	if g.inModule {
		return g.typeErr
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return err
	}
	conf := types.Config{Importer: importer.Default()}
	_, err = conf.Check(file.Name.Name, fset, []*ast.File{file}, nil)
	return err
}

func findGoModRoot(start string) string {
	dir := start
	for {
		if fi, serr := os.Stat(filepath.Join(dir, "go.mod")); serr == nil && !fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
