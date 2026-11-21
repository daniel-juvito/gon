package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The module context of `gon build <dir>` (directory mode only).
//
// `gon build` transpiles every package and then runs `go build`, so when go
// build is certain to fail for want of a module that contains the operand, the
// .go files were already written (RFC M9 T4, amended). checkBuildContext asks
// first and refuses before anything is transpiled, but only when it can PROVE
// one of two conditions; every doubt is "ok" and go build decides as before:
//
//   - no module: `go env GOMOD` is the null device, so go build finds no main
//     module from the directory it runs in;
//   - outside: a module is in force, and the operand is in no module at all, or
//     in a different one (a nested or a sibling module).
//
// It never reads go.work (an active workspace is "ok"), never validates a
// go.mod, and treats a failing `go env`, a failing stat or a failing
// EvalSymlinks as "not proven".

// buildContext is the verdict of the preflight.
type buildContext int

const (
	buildContextOK       buildContext = iota // not provably wrong: go build decides
	buildContextNoModule                     // go finds no main module here
	buildContextOutside                      // the operand is provably outside the main module go uses
)

// message is the text after "gon: cannot build <operand>: ".
func (c buildContext) message() string {
	switch c {
	case buildContextNoModule:
		return "go build requires a module, but no go.mod was found in the current directory or its parents"
	case buildContextOutside:
		return "the package is outside the main module used by go build"
	}
	return ""
}

// hasChdirFlag reports whether a token of flags is -C or --C, alone or as
// -C=dir. It is a conservative reading of tokens, not a check that go accepts
// the form: a token that merely contains "-C" (-ldflags=-C, -tags=C) is not -C.
// The only mistake it can make is to skip the preflight.
func hasChdirFlag(flags []string) bool {
	for _, f := range flags {
		if len(f) < 2 || f[0] != '-' {
			continue
		}
		name := strings.TrimPrefix(f[1:], "-")
		if name == "C" || strings.HasPrefix(name, "C=") {
			return true
		}
	}
	return false
}

// moduleProof is what could be established about the module of a directory.
type moduleProof int

const (
	moduleUnknown moduleProof = iota // could not be established: go decides
	moduleFound                      // a regular file named go.mod, in the root returned: its contents are not looked at
	moduleAbsent                     // dir exists and no ancestor, itself included, has a go.mod file
)

// nearestModule walks up from dir looking for a regular file named go.mod, as
// the go command does (an entry that is not a regular file, such as a
// directory, is passed over). Absence is proven only when dir is a directory
// that stat reaches and the lookup of go.mod fails with "does not exist" (or
// finds a non-file) at dir and at every ancestor up to the root. Any other
// error at any level (permissions, a loop, I/O) is moduleUnknown, and so is
// ENOTDIR (a path component that is not a directory), whether or not the OS
// also reports it as "does not exist": never a claim that there is no module.
func nearestModule(dir string, stat func(string) (os.FileInfo, error)) (string, moduleProof) {
	if fi, err := stat(dir); err != nil || !fi.IsDir() {
		return "", moduleUnknown
	}
	for d := dir; ; {
		fi, err := stat(filepath.Join(d, "go.mod"))
		switch {
		case err == nil && fi.Mode().IsRegular():
			return d, moduleFound
		case errors.Is(err, syscall.ENOTDIR):
			// Before "does not exist": on Windows ENOTDIR also satisfies
			// fs.ErrNotExist, and a path through a file says nothing about
			// whether a go.mod is there.
			return "", moduleUnknown
		case err == nil, errors.Is(err, fs.ErrNotExist):
			// not a go.mod file here: look above
		default:
			return "", moduleUnknown
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", moduleAbsent
		}
		d = parent
	}
}

// parseGoEnv reads the output of `go env GOMOD GOWORK` as one pair: anything
// but exactly two lines is not trusted, so neither value is used on its own.
func parseGoEnv(out string) (gomod, gowork string, ok bool) {
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		return "", "", false
	}
	return lines[0], lines[1], true
}

// decideBuildContext is the pure decision, in this order:
//
//  1. a -C token in flags: ok;
//  2. (the caller) `go env` failed or its output is not the pair: ok;
//  3. GOWORK neither empty nor "off": ok (a workspace may be in force);
//  4. GOMOD empty: ok (GOPATH mode);
//  5. GOMOD is the null device: no module;
//  6. the operand's module could not be established: ok;
//  7. the operand is in no module: outside;
//  8. the operand is in a module: ok if it is the main module's directory,
//     outside if it is another, ok if either directory cannot be resolved;
//  9. otherwise ok.
//
// gomod and gowork are `go env GOMOD GOWORK` printed where go build will run;
// opRoot and op are nearestModule's answer for the operand; resolve is
// filepath.EvalSymlinks in production.
func decideBuildContext(gomod, gowork, opRoot string, op moduleProof, flags []string, resolve func(string) (string, error)) buildContext {
	if hasChdirFlag(flags) {
		return buildContextOK
	}
	if gowork != "" && gowork != "off" {
		return buildContextOK
	}
	if gomod == "" {
		return buildContextOK
	}
	if gomod == os.DevNull {
		return buildContextNoModule
	}
	switch op {
	case moduleUnknown:
		return buildContextOK
	case moduleAbsent:
		return buildContextOutside
	}
	want, e1 := resolve(opRoot)
	have, e2 := resolve(filepath.Dir(gomod))
	if e1 != nil || e2 != nil || want == have {
		return buildContextOK
	}
	return buildContextOutside
}

// checkBuildContext is the preflight for `gon build operand flags...`. go env
// runs where go build will run, with the same environment.
func checkBuildContext(operand string, flags []string) buildContext {
	if hasChdirFlag(flags) {
		return buildContextOK
	}
	out, err := exec.Command("go", "env", "GOMOD", "GOWORK").Output()
	if err != nil {
		return buildContextOK
	}
	gomod, gowork, ok := parseGoEnv(string(out))
	if !ok {
		return buildContextOK
	}
	root := operand
	if r, isPattern := patternRoot(operand); isPattern {
		root = r
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return buildContextOK
	}
	opRoot, proof := nearestModule(abs, os.Stat)
	return decideBuildContext(gomod, gowork, opRoot, proof, flags, filepath.EvalSymlinks)
}
