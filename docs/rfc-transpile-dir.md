# RFC: Directory Form for `gon transpile` and `gon build` (Gon v1.12 / M9)

**Status:** Draft — awaiting owner review. Decisions T1–T9 are *proposals*; §8
lists the questions that need an explicit owner answer. The figures in §2 were
measured on 2026-11-16 (Gon v1.11.0), before any decision.
**Target:** Gon v1.12
**Related:** `docs/rfc-package-check.md` (M8 / v1.10; this RFC is the item M8
left out, its Q-list "no `transpile` / `build` directory form"),
`docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`
**Date:** 2026-11-16

## 1. Summary

Since v1.10, `gon check <dir>` and `gon check ./...` check a directory's
`.gon` files as **one package**. `gon transpile` and `gon build` still accept
one `.gon` **file**, and they type-check that file alone. A file of a
multi-file package that uses a declaration from a sibling file therefore cannot
be transpiled or built: `gon transpile checker.gon` fails with
`undefined: packageShared`, although `gon check ./internal/checker` accepts the
whole package.

This RFC adds the same package form to the commands that **emit** code:

```bash
gon transpile ./internal/foo     # the directory's .gon files -> .go next to each
gon transpile ./...              # every directory below that holds .gon files
gon build ./cmd/app              # transpile, then go build the package
gon transpile file.gon           # unchanged
```

**Normative principle:**

> A package is transpiled only if it checks: the contract diagnostics and the Go
> type check are decided on the **whole package** (the M8 machinery), and a
> package that does not pass leaves **no** file written. The file form is
> unchanged.

## 2. Motivation and measurements

Measured on a copy of this repository (`fd44eb7`, v1.11.0): the 23 non-test
`.go` files of its 8 packages copied to `.gon` (no `!`).

| Measurement | Result |
|---|---|
| `gon check ./...` | 0 errors, exit 0, 5 s (25 GW002, from the repository's own `annotations/os.gna`, `io.gna` — a separate chore) |
| `gon transpile` per file | **9 of 23 succeed; 14 fail** with `Go type error: undefined: <sibling symbol>` |
| The 9 that succeed vs the committed `.go` | **9 byte-identical, 0 different** |
| Which fail | every file of a multi-file package that uses a sibling: all of `internal/checker` (12 files), 2 of `cmd/gon`'s 3, `internal/gna/resolver.go` |
| In-process `go/format` vs `gofmt` on the clean source, against the committed `.go` | **23 of 23 match, both ways** |
| Directory operand today | `gon: input must be a .gon file, got ./internal/checker` (both commands) |

Two more facts from the code (`cmd/gon/main.go`, `transpiler/transpiler.go`):

- `transpile` and `build` already run the contract checker and the Go type
  check, then write `<stem>.go` next to the source (no "generated" header).
- `build` runs `go build -o <stem> <stem>.go`, i.e. it builds **one `.go`
  file**; a multi-file package cannot be built this way even if each file were
  transpiled. `transpiler.Transpile` formats by running `gofmt` **from PATH**
  and, if that fails, silently returns the unformatted source.

So the commands that matter for adopting Gon on a real multi-file codebase
(this repository included) cannot be used on it.

## 3. Decision Matrix

### T1 — `transpile <dir>` / `transpile dir/...`: package mode

Operands are classified exactly as for `check` (a directory, or `dir/...`; a
file operand and every other operand keep the file behaviour). For each
package, the M8 check runs (`CheckPackage`): contract diagnostics for every
file, plus the Go parse / type / package errors of the real package with each
file's clean source overlaid. `*_test.gon` is excluded, as in `check`.

### T2 — all-or-nothing per package

If the package has an **error** diagnostic or a Go type error, the command
exits 1, prints the diagnostics in the human format of `check` (path-prefixed
file names), and writes **no** `.go` file for that package. Warnings are printed
and do not block. With `./...`, a failing package does not stop the others from
being checked and reported, but, as for each package alone, nothing is written
for the failing one. (The file form already works this way for its one file.)

### T3 — what is written

For every `.gon` of a passing package, `<stem>.go` next to it: the clean source
formatted (T7). It replaces a same-named `.go` (the file `transpile` emitted
before), exactly as M8's overlay does. Other `.go` files are left alone, and no
stale `.go` is deleted. One `transpiled: <path>` line per file, as today.

### T4 — `build <dir>`

`gon build <dir>` = T1–T3 for that package, then `go build [flags] <dir>`; extra
arguments are passed through to `go build`. No `-o` is forced: `go build`'s own
rules apply (a `main` package writes a binary named after the directory in the
current directory; a library package is only compiled). `gon build ./...` is
`transpile ./...` then `go build ./...`. The file form keeps `-o <stem> <stem>.go`.

### T5 — scope

Only the `.gon` files named by the operands' directories; a `.gon` in another
package is not touched. A directory with no non-test `.gon` file is exit 2 with
the `check` wording. `testdata`, `vendor`, `_*` and `.*` are skipped by the
pattern form, as in `check`.

### T6 — exit codes

As the other commands: 0 success (warnings do not fail), 1 contract / type error
or a failing `go build`, 2 invalid invocation or nothing to do, 3 internal
failure. No `--json` for these commands.

### T7 — formatting in process

Format with `go/format` in the `gon` process instead of the `gofmt` found on
`PATH`. Measured identical on 23 of 23 files (§2). Why it belongs here: a CI
gate that regenerates `.go` and compares it with the committed one must give the
same bytes on every machine, and the current code falls back to **unformatted**
source when `PATH` has no `gofmt` or an older one that rejects the syntax Gon's
own toolchain accepts. This also changes the file form's output only in that
failing case.

### T8 — non-goals

1. `gon run`, `gon test`, or a watch mode.
2. A "generated" header comment in the `.go` files.
3. Transpiling `*_test.gon` in directory mode (tests stay `.go`; a
   `*_test.gon` named as a file operand still works).
4. Deleting stale `.go` files, or any staleness detection beyond the CI recipe
   in T9.
5. Cross-package contracts read from `.gon` sources (still the separate, later
   RFC named in M8).
6. A new diagnostic code, a protocol change, or a `.gna` schema change.

### T9 — keeping `.gon` and `.go` in sync (recipe, not a feature)

A repository that commits both can gate on
`gon transpile ./... && git diff --exit-code -- '*.go'`: after T1–T7 this is
deterministic. Gon's own repository would adopt it when it converts its sources.

## 4. Normative Rules

1. **Package, not file.** In directory mode the check and the Go type check are
   done on the whole package, never on one file in isolation.
2. **Nothing written on failure** for that package (T2).
3. **A `.gon` replaces its same-named `.go`**; nothing else is created or
   removed (T3).
4. **`build` passes through to `go build`** and forces nothing (T4).
5. **File form unchanged** (P1 of M8 continues to hold).
6. **Output is machine-independent** (T7).

## 5. Compatibility

Additive: a directory operand was an error for these two commands, so no
working invocation changes. The only observable change to the **file** form is
T7, and only where `PATH` had no usable `gofmt` (the old behaviour was to emit
unformatted source). Classification (R1): not breaking. Ships **solo** as v1.12
(R4): its own regression suite, no checker change.

## 6. Implementation Sketch

- `cmd/gon/package.go` already has `expandCheckOperands`, `gonFilesIn` and the
  package loader call; factor "check a package and keep its clean sources" out
  of `checkPackageDir`, and reuse it from `transpile` / `build`.
- Write after the whole package passes; write each file to a temporary name and
  rename, so a failure part-way does not leave a half-written package.
- `transpiler.Transpile` → `go/format.Source` (returns the error instead of
  falling back silently).
- `build`: after writing, run `go build` with the pass-through arguments and the
  package path; keep the existing file-form branch.

## 7. Test Requirements

1. T1/T2: a multi-file package whose files use each other transpiles; a
   package with a contract error, and one with a Go type error, writes nothing;
   `./...` reports every failing package and writes only the passing ones.
2. T3: a same-named `.go` is replaced, another `.go` is untouched, a stale `.go`
   with no `.gon` stays; `*_test.gon` is skipped.
3. T4: `build <dir>` on a `main` package produces a binary named after the
   directory; flags are passed through; a library package builds without output.
4. T5/T6: no `.gon` (and only `*_test.gon`) is exit 2; pattern skips.
5. T7: output equals `gofmt` on a fixture set, and does not depend on `PATH`
   (run with `PATH` stripped of `gofmt`).
6. Regression: every file-form test of `transpile` / `build` passes unchanged;
   a differential run over this repository's `.gon` files shows identical file
   output.
7. Dogfood: transpiling the 23-file copy of this repository (§2) writes `.go`
   files byte-identical to the committed ones.

## 8. Owner Decisions (2026-11-16)

All six were answered **as proposed**:

1. **Q1 — T2 failure policy: ACCEPT all-or-nothing per package.** All-or-nothing per package (proposed), or write
   the files that pass and report the rest?
2. **Q2 — T4 `build` semantics: ACCEPT pass-through to `go build <dir>`, no forced `-o`.** Pass through to `go build <dir>` and force no
   `-o` (proposed), or keep forcing `-o <name>` as the file form does?
3. **Q3 — T7 in-process `go/format`: ACCEPT.** Include it in this release (proposed),
   or leave the `PATH` `gofmt` and handle the determinism another way?
4. **Q4 — T3 stale `.go` and a header: ACCEPT neither.** No deletion and no "generated" header
   (proposed), or add either?
5. **Q5 — T8.3 tests: ACCEPT `*_test.gon` excluded, tests stay `.go`.** `*_test.gon` excluded from directory mode, tests stay
   `.go` (proposed)?
6. **Q6 — Release shape: ACCEPT solo v1.12.** Solo v1.12 (proposed), or fold into the first
   release that converts Gon's own sources?

## 9. Alternatives Considered

- **Keep per-file `transpile` and make the per-file type check lenient** (skip
  sibling-dependent errors). Rejected: it would write Go that does not compile,
  contradicting the contract that a transpiled file passed the check.
- **Teach `go build` about `.gon`** (a `-overlay` or toolexec). Interesting but
  a much larger surface and independent of the checker; out of scope.
- **Only make `build` accept a directory.** `build` needs the transpile step on
  every file first, so it cannot be done without T1–T3.

## 10. Invariant Check (I1–I7)

- **I1, I2, I3, I6** — untouched: no checker rule changes.
- **I4** — untouched: no `.gna` change.
- **I5** — upheld: code generation is still "remove the `!`, format"; no runtime
  behaviour is added.
- **I7** — upheld: no new syntax; schema stays **1**.

## 11. Implementation Notes (v1.12)

- **Where.** `cmd/gon/transpile_dir.go` (`runEmitDir`, `emitPackage`,
  `writePackage`, `orderByDependencies`); `loadPackage` in `cmd/gon/package.go`
  is the M8 check factored out of `checkPackageDir` and shared by `check`,
  `transpile` and `build`. `transpiler.Transpile` is `go/format.Source`.
- **A decision the RFC did not make: package order.** `./...` used to be
  alphabetical. Measured on a copy of this repository with **no `.go` at all**,
  alphabetical order failed (47 type errors, 5 files written), because a package
  was checked against the `.go` of a dependency that had not been written yet.
  The packages of a run are now ordered by the module-local imports of their
  `.gon` sources (Kahn's algorithm, ties broken by the original order; outside a
  module the order is unchanged). Result on the same copy: **0 errors, 23 files
  written, all byte-identical to the committed `.go`, and `go build ./...`
  passes.** With the `.go` already present (the usual case) every order works.
- **Atomicity.** All files of a package are formatted first; each is then
  written to a **uniquely named temporary file created exclusively** in the
  destination directory (`os.CreateTemp`, `.<name>.go.gon-*`, mode set to 0644)
  and renamed into place. The name is never derived from a file that may already
  exist, and only temporaries this call created are removed on failure. (The
  first implementation used the fixed name `<out>.gon-tmp`, which silently
  overwrote a user's file of that name; found in the audit and fixed, with
  `TestTranspileDirDoesNotTouchAFileNamedLikeTheOldTemporary`.) A failure part-way
  through the renames can still leave some files replaced (no cross-file
  transaction on a plain filesystem); the check has already passed by then, so
  the files are valid.
- **`build`** passes the target to `go build` as a directory pattern (`pkg` →
  `./pkg`, since `go build pkg` reads an import path). It builds only if every
  package passed. For a pattern with several `main` packages `go build` itself
  writes nothing, as for plain Go.
- **Measured.** On the 23 non-test `.go` files of this repository copied to
  `.gon`: `gon transpile ./...` with the `.go` present → 23 of 23 byte-identical,
  exit 0; from a tree with no `.go` → 23 of 23 byte-identical, `go build ./...`
  passes. File form, 56 `.gon` files, v1.11.0 against this release: output,
  exit code and emitted `.go` identical.
- **Not part of this RFC, found on the way.** `return Box{P: nil}` — a composite
  literal as the operand of a `return` — is not checked for a nil in a `!` field
  (`_ = Box{P: nil}` and `b := Box{P: nil}` are). Identical on v1.10.0 and
  v1.11.0, so it is not a regression; it is a separate gap in the checker.
