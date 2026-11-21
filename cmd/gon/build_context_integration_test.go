package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// `gon build <dir>` refuses, before anything is transpiled, when go build is
// provably certain to fail for want of a module that contains the operand: exit
// 2, nothing on stdout, nothing written, one of two messages. Every doubt is
// left to go build. Directory mode only.

const (
	bcMain   = "package main\n\nfunc main() {}\n"
	bcGN001  = "package main\n\nfunc f() !*int {\n\treturn nil\n}\n\nfunc main() {}\n"
	bcModule = "module example.com/%s\n\ngo 1.21\n"

	bcMsgNoModule = "go build requires a module, but no go.mod was found in the current directory or its parents"
	bcMsgOutside  = "the package is outside the main module used by go build"
)

// bcTree writes the layout of the matrix under a directory with no go.mod
// above it and returns it:
//
//	free/x           a package in no module
//	m                a module; m/cmd/app a package; m/in a nested module (m/in/cmd)
//	b                a sibling module (b/cmd)
//	wsd              a workspace that uses wsd/a only
func bcTree(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"free/x/main.gon":    bcMain,
		"m/go.mod":           strings.Replace(bcModule, "%s", "m", 1),
		"m/cmd/app/main.gon": bcMain,
		"m/in/go.mod":        strings.Replace(bcModule, "%s", "m/in", 1),
		"m/in/cmd/main.gon":  bcMain,
		"b/go.mod":           strings.Replace(bcModule, "%s", "b", 1),
		"b/cmd/main.gon":     bcMain,
		"wsd/go.work":        "go 1.21\n\nuse ./a\n",
		"wsd/a/go.mod":       strings.Replace(bcModule, "%s", "wsd/a", 1),
		"wsd/a/main.gon":     bcMain,
	}
	root := outsideModule(t, files)
	return root
}

// bcRun runs gon in dir. cwd is restored by chdir's cleanup.
func bcRun(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	chdir(t, dir)
	return captureRun(args)
}

func bcRefusal(op, msg string) string {
	return "gon: cannot build " + op + ": " + msg + "\ngon: nothing was written\n"
}

// writtenGo lists the .go files under root (the tree starts with none).
func writtenGo(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && strings.HasSuffix(p, ".go") {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// V1, V2: no module where go build runs. The operand is in no module (V1) or
// in one elsewhere (V2): either way go build has no main module. Flags that
// merely look like -C do not change that.
func TestBuildDirRefusesWhenThereIsNoModule(t *testing.T) {
	root := bcTree(t)
	free := filepath.Join(root, "free")
	for _, tc := range []struct {
		name string
		args []string
		op   string
	}{
		{"V1 a package", []string{"build", "./x"}, "./x"},
		{"V1 a pattern", []string{"build", "./..."}, "./..."},
		{"V2 a package in a module elsewhere", []string{"build", "../m/cmd/app"}, "../m/cmd/app"},
		{"V1 -ldflags=-C is not -C", []string{"build", "./x", "-ldflags=-C"}, "./x"},
		{"V1 -tags=C is not -C", []string{"build", "./x", "-tags=C"}, "./x"},
		{"V1 -o is passed on, not a reason to skip", []string{"build", "./x", "-o", "out.bin"}, "./x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exit, out, errOut := bcRun(t, free, tc.args...)
			if exit != 2 || out != "" || errOut != bcRefusal(tc.op, bcMsgNoModule) {
				t.Fatalf("exit %d (want 2), stdout %q\nstderr %q\nwant   %q", exit, out, errOut, bcRefusal(tc.op, bcMsgNoModule))
			}
			if got := writtenGo(t, root); len(got) != 0 {
				t.Fatalf("nothing may be written: %v", got)
			}
			if _, err := os.Stat(filepath.Join(free, "out.bin")); err == nil {
				t.Fatal("go build must not have run")
			}
		})
	}
}

// V3 (nested module), V3c (sibling module), V3b (a real directory in no module
// at all): a module is in force, the operand is not in it. The message must not
// claim that go.mod is missing.
func TestBuildDirRefusesAnOperandOutsideTheMainModule(t *testing.T) {
	root := bcTree(t)
	m := filepath.Join(root, "m")
	for _, tc := range []struct{ name, op string }{
		{"V3 nested module", "./in/cmd"},
		{"V3c sibling module", "../b/cmd"},
		{"V3b a directory in no module", "../free/x"},
		{"V3 nested module pattern", "./in/..."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exit, out, errOut := bcRun(t, m, "build", tc.op)
			if exit != 2 || out != "" || errOut != bcRefusal(tc.op, bcMsgOutside) {
				t.Fatalf("exit %d (want 2), stdout %q\nstderr %q\nwant   %q", exit, out, errOut, bcRefusal(tc.op, bcMsgOutside))
			}
			if strings.Contains(errOut, "no go.mod") {
				t.Fatalf("a module is in force, the message must not say go.mod is missing: %q", errOut)
			}
			if got := writtenGo(t, root); len(got) != 0 {
				t.Fatalf("nothing may be written: %v", got)
			}
		})
	}
}

// The verdict comes before the contract diagnostics: a package with a GN001 is
// the context error, not exit 1 with the diagnostic.
func TestBuildDirContextVerdictPrecedesContractDiagnostics(t *testing.T) {
	root := bcTree(t)
	if err := os.WriteFile(filepath.Join(root, "free", "x", "main.gon"), []byte(bcGN001), 0o644); err != nil {
		t.Fatal(err)
	}
	exit, out, errOut := bcRun(t, filepath.Join(root, "free"), "build", "./x")
	if exit != 2 || out != "" || errOut != bcRefusal("./x", bcMsgNoModule) || strings.Contains(errOut, "GN001") {
		t.Fatalf("no module: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	exit, out, errOut = bcRun(t, filepath.Join(root, "m"), "build", "../free/x")
	if exit != 2 || out != "" || errOut != bcRefusal("../free/x", bcMsgOutside) || strings.Contains(errOut, "GN001") {
		t.Fatalf("outside: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// A refusal leaves everything as it was, including a .go beside the .gon.
func TestBuildDirRefusalLeavesAnExistingGoFileAlone(t *testing.T) {
	root := bcTree(t)
	const old = "package main\n\n// the previous output\nfunc main() {}\n"
	path := filepath.Join(root, "free", "x", "main.go")
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	then := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, then, then); err != nil {
		t.Fatal(err)
	}
	if exit, _, _ := bcRun(t, filepath.Join(root, "free"), "build", "./x"); exit != 2 {
		t.Fatalf("exit %d, want 2", exit)
	}
	got, err := os.ReadFile(path)
	fi, serr := os.Stat(path)
	if err != nil || serr != nil || string(got) != old || !fi.ModTime().Equal(then) {
		t.Fatalf("main.go changed: %q, mtime %v", got, fi.ModTime())
	}
}

// What is not proven is left to go build, exactly as before: a missing, a
// looping or a dangling operand is the error of the existing path, never the
// context verdict.
func TestBuildDirDoesNotPredictWhatItCannotProve(t *testing.T) {
	root := bcTree(t)
	free := filepath.Join(root, "free")
	if err := os.Symlink("loop2", filepath.Join(free, "loop1")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink("loop1", filepath.Join(free, "loop2")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.Symlink("nowhere", filepath.Join(free, "dangling")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	for _, op := range []string{"../free/missing/...", "../free/loop1/...", "../free/dangling/..."} {
		t.Run(op, func(t *testing.T) {
			exit, out, errOut := bcRun(t, filepath.Join(root, "m"), "build", op)
			if strings.Contains(errOut, "cannot build") || strings.Contains(errOut, "nothing was written") {
				t.Fatalf("not proven, must not be the verdict: %q", errOut)
			}
			if exit != 2 || out != "" || errOut == "" {
				t.Fatalf("the existing error path is exit 2 with a message: exit %d, stdout %q, stderr %q", exit, out, errOut)
			}
		})
	}
}

// In the module go build uses, build is unchanged: transpile, then go build.
func TestBuildDirInTheMainModuleIsUnchanged(t *testing.T) {
	root := bcTree(t)
	m := filepath.Join(root, "m")
	exit, out, errOut := bcRun(t, m, "build", "./cmd/app")
	if exit != 0 || !strings.Contains(out, "transpiled: "+filepath.FromSlash("cmd/app/main.go")) || !strings.Contains(out, "built: ./cmd/app") || errOut != "" {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", exit, out, errOut)
	}
	if exit, out, errOut := bcRun(t, m, "build", "./..."); exit != 0 || !strings.Contains(out, "built: ./...") {
		t.Fatalf("pattern: exit %d\nstdout %q\nstderr %q", exit, out, errOut)
	}
}

// A go build that really runs and fails stays exit 1 with its own message: exit
// 2 is only for the two conditions of the preflight (RFC M9 T6).
func TestBuildDirGoBuildFailureStaysExit1(t *testing.T) {
	root := bcTree(t)
	exit, out, errOut := bcRun(t, filepath.Join(root, "m"), "build", "./cmd/app", "-gon-no-such-flag")
	if exit != 1 || !strings.Contains(errOut, "gon: go build failed:") || strings.Contains(errOut, "cannot build") {
		t.Fatalf("exit %d (want 1), stdout %q\nstderr %q", exit, out, errOut)
	}
}

// An active workspace is not read: no prediction, so an operand outside it
// still fails in go build after the transpile, as before (a false negative the
// preflight accepts on purpose). GOWORK=off is no workspace.
func TestBuildDirWorkspaces(t *testing.T) {
	root := bcTree(t)
	exit, _, errOut := bcRun(t, filepath.Join(root, "wsd"), "build", "../free/x")
	if exit != 1 || !strings.Contains(errOut, "gon: go build failed:") || strings.Contains(errOut, "cannot build") {
		t.Fatalf("workspace: exit %d (want 1: go build decides)\nstderr %q", exit, errOut)
	}
	if got := writtenGo(t, root); len(got) != 1 || got[0] != "free/x/main.go" {
		t.Fatalf("the transpile happened before go build failed, as before: %v", got)
	}

	root = bcTree(t)
	t.Setenv("GOWORK", "off")
	exit, out, errOut := bcRun(t, filepath.Join(root, "wsd", "a"), "build", "../../free/x")
	if exit != 2 || out != "" || errOut != bcRefusal("../../free/x", bcMsgOutside) {
		t.Fatalf("GOWORK=off: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	exit, out, errOut = bcRun(t, filepath.Join(root, "free"), "build", "./x")
	if exit != 2 || out != "" || errOut != bcRefusal("./x", bcMsgNoModule) {
		t.Fatalf("GOWORK=off, no module: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// -C changes the directory go build runs in, which the preflight does not
// follow: no prediction.
func TestBuildDirChdirFlagIsLeftToGo(t *testing.T) {
	for _, flag := range [][]string{{"-C", "../m"}, {"-C=../m"}} {
		root := bcTree(t)
		exit, out, errOut := bcRun(t, filepath.Join(root, "free"), append([]string{"build", "../m/cmd/app"}, flag...)...)
		if exit != 0 || strings.Contains(errOut, "cannot build") {
			t.Fatalf("%v: exit %d, stdout %q\nstderr %q", flag, exit, out, errOut)
		}
	}
}

// GOPATH mode (GO111MODULE=off): GOMOD is empty, no prediction.
func TestBuildDirGOPATHModeIsLeftToGo(t *testing.T) {
	root := bcTree(t)
	t.Setenv("GO111MODULE", "off")
	exit, out, errOut := bcRun(t, filepath.Join(root, "free"), "build", "./x", "-o", "out.bin")
	if exit != 0 || !strings.Contains(out, "built: ./x") || strings.Contains(errOut, "cannot build") {
		t.Fatalf("exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// Only directory-mode build has the preflight: transpile, a file-mode build and
// check from a directory with no module are unchanged.
func TestBuildContextPreflightIsOnlyForDirectoryBuild(t *testing.T) {
	root := bcTree(t)
	free := filepath.Join(root, "free")
	if exit, out, _ := bcRun(t, free, "check", "./x"); exit != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("check: exit %d, stdout %q", exit, out)
	}
	if exit, out, errOut := bcRun(t, free, "build", "x/main.gon"); exit != 0 || !strings.Contains(out, "built: main") || errOut != "" {
		t.Fatalf("file-mode build: exit %d\nstdout %q\nstderr %q", exit, out, errOut)
	}
	_ = os.Remove(filepath.Join(free, "x", "main.go"))
	_ = os.Remove(filepath.Join(free, "main"))
	if exit, out, errOut := bcRun(t, free, "transpile", "./x"); exit != 0 || !strings.Contains(out, "transpiled: "+filepath.FromSlash("x/main.go")) || errOut != "" {
		t.Fatalf("transpile: exit %d\nstdout %q\nstderr %q", exit, out, errOut)
	}
}

// Operands that reach a module through a symlink. go build joins the operand
// to the working directory lexically and asks which module holds the result:
//
//	A  lexically in the main module, physically in no module: go builds it;
//	B  lexically in the main module, physically in another module: go refuses;
//	D  from a directory with no module, through a link into a module: no module.
//
// The preflight agrees with go in all three: A is not refused, B and D are.
func TestBuildDirSymlinkedOperands(t *testing.T) {
	root := bcTree(t)
	for _, l := range [][2]string{
		{"../free", filepath.Join(root, "m", "tofree")},
		{"../b", filepath.Join(root, "m", "tob")},
		{"../m", filepath.Join(root, "free", "tom")},
	} {
		if err := os.Symlink(l[0], l[1]); err != nil {
			t.Skipf("symlink: %v", err)
		}
	}
	m, free := filepath.Join(root, "m"), filepath.Join(root, "free")

	exit, out, errOut := bcRun(t, m, "build", "./tofree/x")
	if exit != 0 || !strings.Contains(out, "built: ./tofree/x") || strings.Contains(errOut, "cannot build") {
		t.Fatalf("A: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	exit, out, errOut = bcRun(t, m, "build", "./tob/cmd")
	if exit != 2 || out != "" || errOut != bcRefusal("./tob/cmd", bcMsgOutside) {
		t.Fatalf("B: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	exit, out, errOut = bcRun(t, free, "build", "./tom/cmd/app")
	if exit != 2 || out != "" || errOut != bcRefusal("./tom/cmd/app", bcMsgNoModule) {
		t.Fatalf("D: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
}

// The working directory is a symlink to the module and the operand is its
// physical absolute path: go refuses it ("outside main module") but the two
// spellings name the same directory once symlinks are resolved, so the
// preflight does not claim a mismatch it cannot prove. go build decides, after
// the transpile, as before (a false negative on purpose).
func TestBuildDirSameDirectoryUnderTwoSpellingsIsNotRefused(t *testing.T) {
	root := bcTree(t)
	link := filepath.Join(root, "linkm")
	if err := os.Symlink("m", link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	physical, err := filepath.EvalSymlinks(filepath.Join(root, "m", "cmd", "app"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PWD", link) // the logical working directory, as a shell sets it
	exit, out, errOut := bcRun(t, link, "build", physical)
	if strings.Contains(errOut, "cannot build") || strings.Contains(errOut, "nothing was written") {
		t.Fatalf("not provable, must not be the verdict: exit %d, stdout %q\nstderr %q", exit, out, errOut)
	}
	if exit != 1 || !strings.Contains(errOut, "gon: go build failed:") {
		t.Fatalf("go build decides: exit %d (want 1), stdout %q\nstderr %q", exit, out, errOut)
	}
}
