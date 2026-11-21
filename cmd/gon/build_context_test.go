package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The preflight of `gon build <dir>`: pure decisions. They are named for what
// they pin; the integration side is in build_context_integration_test.go.

type fakeInfo struct {
	name string
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

// fakeFS: dirs are directories, files regular files, failures map a path to an
// error; every other path does not exist.
func fakeFS(dirs, files []string, failures map[string]error) func(string) (os.FileInfo, error) {
	return func(p string) (os.FileInfo, error) {
		if err, ok := failures[p]; ok {
			return nil, err
		}
		for _, d := range dirs {
			if p == d {
				return fakeInfo{p, fs.ModeDir}, nil
			}
		}
		for _, f := range files {
			if p == f {
				return fakeInfo{p, 0}, nil
			}
		}
		return nil, &fs.PathError{Op: "stat", Path: p, Err: syscall.ENOENT}
	}
}

func slashes(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = filepath.FromSlash(x)
	}
	return out
}

func slashKeys(m map[string]error) map[string]error {
	out := map[string]error{}
	for k, v := range m {
		out[filepath.FromSlash(k)] = v
	}
	return out
}

func TestHasChdirFlag(t *testing.T) {
	for _, tc := range []struct {
		flags []string
		want  bool
	}{
		{[]string{"-C", "."}, true}, {[]string{"-C=."}, true}, {[]string{"--C"}, true}, {[]string{"--C=x"}, true},
		{[]string{"-o", "out", "-C", "d"}, true},
		{[]string{"-ldflags=-C"}, false}, {[]string{"-tags=C"}, false}, {[]string{"-o", "out"}, false},
		{[]string{"-Cx"}, false}, {[]string{"C"}, false}, {[]string{"-"}, false}, {[]string{"--"}, false}, {nil, false},
	} {
		if got := hasChdirFlag(tc.flags); got != tc.want {
			t.Errorf("hasChdirFlag(%q) = %v, want %v", tc.flags, got, tc.want)
		}
	}
}

func TestNearestModule(t *testing.T) {
	eacces := &fs.PathError{Op: "stat", Err: syscall.EACCES}
	eio := &fs.PathError{Op: "stat", Err: syscall.EIO}
	eloop := &fs.PathError{Op: "stat", Err: syscall.ELOOP}
	enotdir := &fs.PathError{Op: "stat", Err: syscall.ENOTDIR}
	dirs := []string{"/", "/w", "/w/m", "/w/m/cmd", "/w/m/cmd/app", "/w/free", "/w/free/x"}
	for _, tc := range []struct {
		name     string
		dir      string
		files    []string
		failures map[string]error
		wantRoot string
		want     moduleProof
	}{
		{"nearest go.mod above", "/w/m/cmd/app", []string{"/w/m/go.mod"}, nil, "/w/m", moduleFound},
		{"go.mod in the dir itself", "/w/m", []string{"/w/m/go.mod"}, nil, "/w/m", moduleFound},
		{"nested: the inner one wins", "/w/m/cmd/app", []string{"/w/m/go.mod", "/w/m/cmd/go.mod"}, nil, "/w/m/cmd", moduleFound},
		{"no go.mod anywhere above an existing dir", "/w/free/x", nil, nil, "", moduleAbsent},
		{"the dir does not exist", "/w/free/missing", nil, nil, "", moduleUnknown},
		{"the path is a file, not a dir", "/w/m/go.mod", []string{"/w/m/go.mod"}, nil, "", moduleUnknown},
		{"stat of the dir fails (loop)", "/w/free/x", nil, map[string]error{"/w/free/x": eloop}, "", moduleUnknown},
		{"go.mod lookup denied", "/w/free/x", nil, map[string]error{"/w/free/x/go.mod": eacces}, "", moduleUnknown},
		{"go.mod lookup denied in an ancestor", "/w/free/x", nil, map[string]error{"/w/free/go.mod": eacces}, "", moduleUnknown},
		{"go.mod lookup I/O error above", "/w/free/x", nil, map[string]error{"/w/go.mod": eio}, "", moduleUnknown},
		{"go.mod lookup not a directory", "/w/free/x", nil, map[string]error{"/w/free/go.mod": enotdir}, "", moduleUnknown},
		// the nearest go.mod decides: an error above it is never reached
		{"error above a found go.mod is not reached", "/w/m/cmd", []string{"/w/m/go.mod"}, map[string]error{"/w/go.mod": eio}, "/w/m", moduleFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, got := nearestModule(filepath.FromSlash(tc.dir), fakeFS(slashes(dirs), slashes(tc.files), slashKeys(tc.failures)))
			if got != tc.want || root != filepath.FromSlash(tc.wantRoot) {
				t.Fatalf("got (%q, %v), want (%q, %v)", root, got, tc.wantRoot, tc.want)
			}
		})
	}
}

// ENOENT for go.mod at the directory and at every ancestor is the only road to
// moduleAbsent; any other error at any single level is moduleUnknown, never a
// claim that no module exists.
func TestNearestModuleOnlyConsistentENOENTIsAbsent(t *testing.T) {
	dirs := slashes([]string{"/", "/w", "/w/free", "/w/free/x"})
	dir := filepath.FromSlash("/w/free/x")

	root, got := nearestModule(dir, fakeFS(dirs, nil, nil))
	if got != moduleAbsent || root != "" {
		t.Fatalf("ENOENT at every level: got (%q, %v), want absent", root, got)
	}

	errs := map[string]error{
		"ENOTDIR": &fs.PathError{Op: "stat", Err: syscall.ENOTDIR},
		"EACCES":  &fs.PathError{Op: "stat", Err: syscall.EACCES},
		"EIO":     &fs.PathError{Op: "stat", Err: syscall.EIO},
		"ELOOP":   &fs.PathError{Op: "stat", Err: syscall.ELOOP},
	}
	for _, d := range dirs {
		for name, e := range errs {
			gm := filepath.Join(d, "go.mod")
			t.Run(name+" at "+gm, func(t *testing.T) {
				root, got := nearestModule(dir, fakeFS(dirs, nil, map[string]error{gm: e}))
				if got != moduleUnknown || root != "" {
					t.Fatalf("got (%q, %v), want unknown", root, got)
				}
			})
		}
	}
	// A go.mod that is a directory is not a module file; like the go command
	// the search goes on to the parent, so it is "absent" only if nothing else
	// is found up to the root.
	_, got = nearestModule(dir, fakeFS(append(slashes([]string{"/w/free/go.mod"}), dirs...), nil, nil))
	if got != moduleAbsent {
		t.Fatalf("go.mod as a directory, nothing above: got %v, want absent", got)
	}
	root, got = nearestModule(dir, fakeFS(append(slashes([]string{"/w/free/go.mod"}), dirs...), slashes([]string{"/w/go.mod"}), nil))
	if got != moduleFound || root != filepath.FromSlash("/w") {
		t.Fatalf("go.mod as a directory, a file above: got (%q, %v), want /w found", root, got)
	}
}

// notDirAndNotExist is the error Windows gives for a path through a file: it is
// both "not a directory" and "does not exist" to errors.Is. Only the first is
// the truth about the lookup; the second must not be read as proof that no
// go.mod is there.
type notDirAndNotExist struct{}

func (notDirAndNotExist) Error() string { return "stat: not a directory (and does not exist)" }
func (notDirAndNotExist) Is(target error) bool {
	return target == fs.ErrNotExist || target == syscall.ENOTDIR
}

// ENOTDIR is classified before "does not exist", on every OS: an error that is
// both is moduleUnknown at any level of the search, never the road to absent.
func TestNearestModuleENOTDIRIsCheckedBeforeNotExist(t *testing.T) {
	dirs := slashes([]string{"/", "/w", "/w/free", "/w/free/x"})
	dir := filepath.FromSlash("/w/free/x")
	for _, d := range dirs {
		gm := filepath.Join(d, "go.mod")
		t.Run(gm, func(t *testing.T) {
			root, got := nearestModule(dir, fakeFS(dirs, nil, map[string]error{gm: notDirAndNotExist{}}))
			if got != moduleUnknown || root != "" {
				t.Fatalf("got (%q, %v), want unknown: an error that is also ENOTDIR is not proof of absence", root, got)
			}
		})
	}
	// A plain does-not-exist at every level is still the road to absent.
	if _, got := nearestModule(dir, fakeFS(dirs, nil, nil)); got != moduleAbsent {
		t.Fatalf("plain ENOENT at every level: got %v, want absent", got)
	}
}

// go env GOMOD GOWORK is one pair: anything but exactly two lines is not used,
// neither value on its own.
func TestParseGoEnv(t *testing.T) {
	for _, tc := range []struct {
		name, out     string
		gomod, gowork string
		ok            bool
	}{
		{"module, no workspace (empty second line)", "/w/m/go.mod\n\n", "/w/m/go.mod", "", true},
		{"no module", os.DevNull + "\n\n", os.DevNull, "", true},
		{"workspace", "/w/m/go.mod\n/w/go.work\n", "/w/m/go.mod", "/w/go.work", true},
		{"workspace off", os.DevNull + "\noff\n", os.DevNull, "off", true},
		{"both empty (GOPATH mode)", "\n\n", "", "", true},
		{"one line with a newline", "/w/m/go.mod\n", "", "", false},
		{"one line only", "/w/m/go.mod", "", "", false},
		{"nothing", "", "", "", false},
		{"three lines", "a\nb\nc\n", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gomod, gowork, ok := parseGoEnv(tc.out)
			if ok != tc.ok || gomod != tc.gomod || gowork != tc.gowork {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", gomod, gowork, ok, tc.gomod, tc.gowork, tc.ok)
			}
		})
	}
}

// The nine rules, in order.
func TestDecideBuildContext(t *testing.T) {
	same := func(p string) (string, error) { return p, nil }
	fails := func(p string) (string, error) { return "", errors.New("resolve failed") }
	failsFor := func(bad string) func(string) (string, error) {
		return func(p string) (string, error) {
			if p == bad {
				return "", errors.New("resolve failed")
			}
			return p, nil
		}
	}
	const modA = "/w/a"
	gomodA := filepath.Join(filepath.FromSlash(modA), "go.mod")
	root := func(s string) string { return filepath.FromSlash(s) }
	for _, tc := range []struct {
		name          string
		gomod, gowork string
		opRoot        string
		op            moduleProof
		flags         []string
		resolve       func(string) (string, error)
		want          buildContext
	}{
		{"1 -C wins over a missing module", os.DevNull, "", "", moduleAbsent, []string{"-C", "x"}, same, buildContextOK},
		{"1 -C= wins over a mismatch", gomodA, "", root("/w/b"), moduleFound, []string{"-C=x"}, same, buildContextOK},
		{"1 -ldflags=-C does not skip", os.DevNull, "", "", moduleAbsent, []string{"-ldflags=-C"}, same, buildContextNoModule},
		{"3 workspace path, no module", os.DevNull, "/w/go.work", "", moduleAbsent, nil, same, buildContextOK},
		{"3 workspace path, module mismatch", gomodA, "/w/go.work", root("/w/b"), moduleFound, nil, same, buildContextOK},
		{"3 workspace off: a module is still needed", os.DevNull, "off", "", moduleAbsent, nil, same, buildContextNoModule},
		{"3 workspace off, same module", gomodA, "off", root(modA), moduleFound, nil, same, buildContextOK},
		{"4 GOMOD empty (GOPATH mode)", "", "", "", moduleAbsent, nil, same, buildContextOK},
		{"4 GOMOD empty and a workspace", "", "/w/go.work", "", moduleAbsent, nil, same, buildContextOK},
		{"5 no module, operand in no module", os.DevNull, "", "", moduleAbsent, nil, same, buildContextNoModule},
		{"5 no module, operand in a module (V2)", os.DevNull, "", root("/w/m"), moduleFound, nil, same, buildContextNoModule},
		{"5 no module comes before operand status", os.DevNull, "", "", moduleUnknown, nil, same, buildContextNoModule},
		{"6 operand module unknown", gomodA, "", "", moduleUnknown, nil, same, buildContextOK},
		{"7 operand in no module (V3b)", gomodA, "", "", moduleAbsent, nil, same, buildContextOutside},
		{"8 same module", gomodA, "", root(modA), moduleFound, nil, same, buildContextOK},
		{"8 nested module (V3)", gomodA, "", root("/w/a/in"), moduleFound, nil, same, buildContextOutside},
		{"8 sibling module (V3c)", gomodA, "", root("/w/b"), moduleFound, nil, same, buildContextOutside},
		{"8 resolve fails for both", gomodA, "", root("/w/b"), moduleFound, nil, fails, buildContextOK},
		{"8 resolve fails for the operand root", gomodA, "", root("/w/b"), moduleFound, nil, failsFor(root("/w/b")), buildContextOK},
		{"8 resolve fails for the main module dir", gomodA, "", root("/w/b"), moduleFound, nil, failsFor(root(modA)), buildContextOK},
		{"8 symlinked operand resolves to the main module", gomodA, "", root("/w/link"), moduleFound, nil,
			func(p string) (string, error) {
				if p == root("/w/link") {
					return root(modA), nil
				}
				return p, nil
			}, buildContextOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideBuildContext(tc.gomod, tc.gowork, tc.opRoot, tc.op, tc.flags, tc.resolve); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Two distinct messages; the second must not claim that go.mod is missing
// (the module is there, the operand is not in it).
func TestBuildContextMessages(t *testing.T) {
	if got, want := buildContextNoModule.message(), "go build requires a module, but no go.mod was found in the current directory or its parents"; got != want {
		t.Errorf("no module: %q", got)
	}
	if got, want := buildContextOutside.message(), "the package is outside the main module used by go build"; got != want {
		t.Errorf("outside: %q", got)
	}
	if buildContextOK.message() != "" {
		t.Error("ok has no message")
	}
}
