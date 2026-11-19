# RFC: Package-Aware Checking (Gon v1.10 / M8)

**Status:** Implemented in Gon v1.10 (2026-11-15). Owner decisions Q1–Q7
were recorded the same day, each taking the drafted recommendation; §14 lists
what the implementation did and where it differs from the sketch. The baseline
and feasibility figures in §12 were measured before any decision.
**Target:** Gon v1.10
**Related:** `docs/v1-scope.md`, `docs/roadmap-v1.3-plus.md`,
`docs/diagnostic-protocol-v1.md`, `docs/rfc-ecosystem-contract-expansion.md`
(M4/M5a / v1.5), `docs/rfc-field-contracts.md` (v1.2),
`docs/rfc-return-value-contracts.md` (v1.1); interacts with
`docs/rfc-method-contracts.md` (M7, proposed — its S9 scope limit)
**Date:** 2026-11-15

## 1. Summary

`gon check file.gon` type-checks the file as a **synthetic one-file package**
(`__gon_overlay__/check.go` at the module root) and builds every contract table
from that one file. That is right for a self-contained program and wrong for a
real Go package, which is several files that refer to each other. Measured on
Gon v1.9.1 (§12):

- A file that uses a declaration from a sibling file fails Go type-checking
  (`undefined: helper`). Depending on the file, the contract diagnostics are
  printed first and the run then aborts, or the run aborts before any
  contract diagnostic is produced.
- Two files given to one invocation are processed one after the other, and the
  run **stops at the first failure**: the second file is never checked, and
  `--json` emits a document that contains only the first file's diagnostics.
- A contract written in one file (a `!` parameter, a `!` field, a `!` result)
  is invisible to the other files of the same package.
- In Gon's own source, 8.6% of the references inside a package point at a
  declaration in another file (11.9% in `internal/checker`, which has ten
  files).

This RFC adds an explicit **package mode**: `gon check ./dir` type-checks the
directory's `.gon` files (plus its `.go` files) **together, in their real
location**, shares contract tables across the files, and reports each
diagnostic against its own file. File arguments keep today's single-file
behaviour unchanged.

**Normative principle:**

> A package is checked as a package. Its files are type-checked together where
> they live; a contract declared in any file of the package is visible to every
> file of it; and each diagnostic is reported against the file it belongs to.
> Naming a *file* still means checking that file alone.

## 2. Motivation

1. **The feature is unusable on a real multi-file package.** Anyone with a
   package of more than one file cannot check one file in its real context
   through the CLI. They get a type error, or an aborted run.
2. **Self-hosting.** Gon's product code is plain Go (20 non-test `.go` files, none `.gon`; the
   26 `.gon` files are test fixtures).
   Converting it to `.gon` — the owner's stated goal — is blocked here:
   `internal/checker` is ten files with 11.9% cross-file references, so no
   file of it can be checked alone. (Six of the nine packages are single-file
   and *can* already be checked today; §12.3.)
3. **A scope limit the other RFCs inherit.** `rfc-method-contracts.md` S9
   limits method contracts to "the checked file", and plain functions have the
   same limit. Package mode lifts it for same-package declarations.
4. **Editor and CI use the same entry point.** `gon check --json` is what the
   VS Code extension and CI call; a package-level invocation gives them one
   document for the whole package.

## 3. Terminology

- **Package mode** — an invocation whose argument is a directory (or a
  `./...` pattern), as opposed to a file.
- **Package files** — the `.gon` files of one directory, plus that
  directory's `.go` files, as `go/packages` would load them.
- **Shadowing** — a `.gon` file replaces the `.go` file of the same base name
  (`x.gon` replaces `x.go`) for the check. Gon's `transpile` emits `x.go` next
  to `x.gon`, so in a converted codebase both exist.
- **Synthetic one-file package** — today's behaviour: the file is placed in
  `__gon_overlay__/` at the module root and checked alone.

## 4. Decision Matrix

### P1 — Package mode is opt-in, selected by the argument

An argument that is a **directory** (`gon check ./internal/checker`) or a
`./...` pattern selects package mode. An argument that is a **file** keeps
the current single-file semantics exactly. No flag is needed, and nothing that
works today changes meaning: a directory argument is currently rejected
(`input must be a .gon file`), so the new form is purely additive.

*Why not make a folder of files a package by default.* Folders of independent
programs are common in Gon's own repositories: 4 of 5 files in
`testdata/valid`, 6 of 6 editor fixtures in `gon-vscode`, and both files in two
protocol-fixture folders each declare their own `main()` (§12.4). Checking them
as one package would be wrong.

### P2 — Which files form the package

For a directory: every `*.gon` file, plus the `.go` files `go/packages` would
include (build constraints respected; `*_test.go` excluded, see Q3). A `.gon`
file **shadows** the `.go` file of the same base name. All files must declare
the same package clause; a mismatch is an error naming both files.

### P3 — Type-check in the real directory

The check is a `go/packages` load of the real package with an **overlay at each
file's real path**: the `.gon` file's clean source replaces `x.go` if it
exists, or is added to the directory if it does not. This is the harness that
checked Gon's own 20 product files with full type information and no false
positive (§12.2), made a feature.

### P4 — Contracts are shared across the package's files

The declaration tables (function parameters and results, struct field
contracts) are built from **all** the package's `.gon` files, then every file is
walked. `!` positions are recorded **per file** (they are byte offsets into a
file's clean source), so the offset table becomes `file → offset set`. `.go`
files carry no `!` and so no contracts. Method contracts follow
`rfc-method-contracts.md` by object identity; this RFC only widens "the checked
file" to "the package".

### P5 — Diagnostics are attributed per file, deterministically

Each diagnostic carries its own file and Gon-space position (the per-file
`CleanToOrig` mapping already used for single files). Order: files sorted by
name, then source order inside a file. `--json` emits **one** Diagnostic
Protocol v1 document containing every file's diagnostics; the schema is
unchanged (a diagnostic already has a `file`).

### P6 — Go type errors do not hide contract diagnostics

As for a single file today, contract diagnostics are printed first, then Go
type errors, then the process exits 1. In package mode a type error in one file
no longer prevents the other files from being checked: all files are walked and
all diagnostics reported.

### P7 — Cross-package contracts are unchanged

An import of another package still resolves its contracts from `.gna`, whether
the package is external or module-local. Reading a module-local package's
contracts straight from its `.gon` sources (or generating `.gna` from them) is a
separate feature and **not** part of this RFC (see Q5). Package mode only makes
*same-package* contracts visible.

### P8 — `./...` follows Go's conventions

`./...` walks directories that contain `.gon` files, each its own package,
skipping `testdata`, `vendor`, and names beginning with `_` or `.` — as the
`go` tool does. This keeps the independent fixture folders of §12.4 out of
package mode. *(v1.16: it also stops at a directory with its own `go.mod`, a
nested module, as the `go` tool does; a package none of whose sources is in
the build is skipped.)*

### P9 — Other commands

`fmt` is per-file and unchanged. `transpile` and `build` keep their current
file arguments; a directory form for them is out of scope.

### P10 — Editors

The editor extension keeps calling `gon check --json <file>`: still single-file.
Giving it an opt-in package setting is an extension change, not part of the
language-tool RFC.

### P11 — Compatibility

Additive. File arguments, exit codes, and the Diagnostic Protocol v1 schema do
not change; a directory argument was an error before. No `.gna` change, no new
diagnostic code, no schema bump. Ships **solo** as v1.10 (R1/R4: it adds no
breaking semantics, but it is a distinct feature with its own tests).

### P12 — Non-Goals

1. Changing single-file semantics, or making a folder a package by default (P1).
2. Reading contracts of imported module-local packages from `.gon` source (P7).
3. Test files, build-tag matrices, incremental or watch mode.
4. A directory form for `transpile` / `build` (P9).
5. Any change to what a `!` means; this RFC only changes *where* a check looks.

## 5. Normative Rules

1. **Argument decides the mode.** A file is checked alone; a directory or
   `./...` is checked as packages.
2. **Real location.** In package mode the files are type-checked in their real
   directory, with `.gon` clean sources overlaid on (or added to) the package.
3. **Shadowing.** A `.gon` file replaces the `.go` file of the same base name.
4. **Shared contracts.** A declaration contract written in any `.gon` file of
   the package applies to uses in every `.gon` file of it.
5. **Per-file reporting.** Every diagnostic names its own file and Gon
   position; order is by file name, then position.
6. **No early abort.** A type or contract problem in one file does not stop
   the others from being checked.

## 6. Examples

```
internal/foo/
├── a.gon      // type Conn struct{ DB !*T }; func Take(p !*T) {}
├── b.gon      // uses Take and Conn
└── b_test.go  // ignored in package mode
```

```bash
gon check ./internal/foo          # a and b checked together
gon check ./...                   # every directory with .gon files
gon check internal/foo/b.gon      # unchanged: b alone (and 'undefined: Take')
```

In `b.gon`, `Take(nil)` is GN001 and `Conn{}` is GN002 in package mode, because
`Take` and `Conn` are declared in `a.gon`. Today the same file fails with a type
error.

## 7. Compatibility

See P11. Anything that is a file argument today behaves identically. The only
new surface is the directory and `./...` forms.

## 8. Implementation Sketch

- **Argument classification** (`cmd/gon`): `os.Stat` each argument; a directory
  or a `./...` pattern → package mode; a file → existing path.
- **Loading** (`cmd/gon` / `internal/checker`): list the directory's `.gon`
  files; `preproc.Process` each; build `Overlay` mapping each file's *real* `.go`
  path (the shadowed sibling's path, or `<dir>/<base>.go`) to its clean source;
  one `packages.Load(cfg, "file="+firstOverlayPath)` for the package; take
  `TypesInfo` and the per-file syntax trees.
- **Checker refactor**: introduce a package-level holder (type info, the shared
  declaration tables, and `nonNilOffsets` keyed by file) and a per-file walk
  that reads them. Today `Checker` owns one file, one offset table, and one set
  of tables; `isNonNil` reads `c.nonNilOffsets[offset]` and must become
  `offsets[filename][offset]` (looked up by the node's file via `c.fset`).
- **Declaration collection**: `collectDecls` iterates every `.gon` file's AST
  instead of one.
- **Reporting**: a diagnostic gets its file from the position's `Filename`;
  per-file `CleanToOrig` maps back to Gon space (as `cmd/gon/protocol.go` does
  for one file).
- **Docs / version:** `v1-scope.md`, README (usage and the self-hosting note),
  `diagnostic-protocol-v1.md` (one document, many files — already the shape),
  CHANGELOG, `gon version` → `1.10.0`; examples and the editor fixture follow.

## 9. Test Requirements

Executable spec in `cmd/gon/package_check_test.go` and
`internal/checker/package_test.go`.

1. P1: a file argument behaves identically to v1.9.x on every existing fixture
   (differential run); a directory argument selects package mode; a directory
   of independent `main()` programs is not treated as one package unless
   named as a directory (and then reports the redeclaration, which is correct).
2. P2/P3: two files, each using the other's declarations, produce no `undefined`
   error; `x.gon` shadows `x.go`; a `.gon` with no sibling `.go` is added; build
   constraints respected; `_test.go` excluded; package-clause mismatch reported.
3. P4: a `!` parameter, field and result declared in one file are enforced in
   the other (GN001, GN002, GW001); `!` offsets of different files do not
   collide (same byte offset in two files).
4. P5: diagnostics carry the right file and Gon-space position (including a
   file containing `!` before multi-byte text); order is by file then position;
   `--json` is one document holding all files, schema version 1.
5. P6: a type error in one file does not suppress contract diagnostics in
   another; exit code 1.
6. P8: `./...` skips `testdata`, `vendor`, `_x`, `.x`; finds nested packages.
7. Regression: the 540-file corpus and the repository's `.gon` files produce
   identical output in file mode.
8. A package of Gon's own size (the 10-file `internal/checker` converted with
   no `!`) checks with zero diagnostics — the §12.2 result, as a test.

## 10. Owner Decisions

Q1–Q7 were answered on 2026-11-15, each accepting the drafted
recommendation. They settle the alternatives; they are not implementation
approval or release sign-off.

1. **Q1 — P1: ACCEPT.** A directory or `./...` argument selects package mode;
   no flag is added. File arguments keep today's single-file behaviour
   exactly.
2. **Q2 — P2: ACCEPT.** `x.gon` shadows the `.go` file of the same base name,
   which a converted codebase needs (`transpile` writes `x.go` beside `x.gon`).
3. **Q3 — P2: ACCEPT.** `*_test.go` files are excluded from package mode; the
   scope is the production package.
4. **Q4 — P6: ACCEPT.** Every file's diagnostics are reported even when
   another file has a type error, consistent with Diagnostic Protocol v1 and
   so that no diagnostic is lost.
5. **Q5 — P7: ACCEPT `.gna`.** Contracts of imported packages, module-local or
   external, keep resolving from `.gna`. Package mode widens visibility only
   *within* one package. Reading a module-local package's contracts straight
   from its `.gon` sources (or generating `.gna` from them) is **not rejected
   for good**: it needs its own RFC and is not part of M8. Two large
   architectural changes are not combined in one release.
6. **Q6 — Sequencing: ACCEPT.** M8 ships as **v1.10** and
   `rfc-method-contracts.md` (M7) as **v1.11**, so that M7's single-file limit
   (its S9) lifts to "the package" before M7 is built.
7. **Q7 — Release shape: ACCEPT solo v1.10.** The feature is additive, isolated
   and has its own regression suite.

## 11. Alternatives Considered

- **Make every folder a package.** Rejected by P1: it would break the many
  folders of independent programs (§12.4).
- **A `gon check --package` flag.** Equivalent in power; the argument form is
  shorter and mirrors `go build ./dir`. Left to Q1.
- **Concatenate a package's `.gon` files into one synthetic file.** Breaks
  per-file positions, build constraints and `init` ordering, and cannot add
  files next to `.go` siblings.
- **Convert `internal/checker` to a single file.** Avoids the feature by
  distorting the code to fit the tool.
- **Convert only the six single-file packages first.** Possible today without
  any change and independent of this RFC (§12.3); it does not help the checker
  package itself.

## 12. Exercise: measurements taken before any decision (2026-11-15)

### 12.1 What the CLI does with several files today

Two files in one directory, `a.gon` declaring `Take`, `Conn`, `main` and
`b.gon` declaring `helper` and using `Take` / `Conn`, run with Gon v1.9.1:

| Invocation | Result |
|---|---|
| `check a.gon` | GN001 and GN002 printed, then `Go type error: undefined: helper` |
| `check b.gon` | `Go type error: undefined: T` — no contract diagnostic at all |
| `check a.gon b.gon` | a's output, then stops at the type error; **b is never checked** |
| `check --json a.gon b.gon` | one document containing **only a.gon's** diagnostics |

### 12.2 Is a package checkable in its real context? (feasibility)

A throwaway test overlaid each of Gon's own non-test source files at its real
path and loaded the whole package with `go/packages`, then ran the checker on
that file:

- **20 files, 5,321 lines; all 20 received full package type information;
  0 diagnostics.**
- **Control:** with a deliberate violation appended to a real file, the same
  harness reported `GN001` (`spikeTake(nil)`) and `GN002` (`spikeBox{}`).

So package-context checking works, and the zero is meaningful: no false
positive on real code of this size.

### 12.3 How much does a one-file check not see?

References from a file to a declaration of the same package, counted from
`go/types` (`Uses`), per package:

| Package | Files | Same-package refs | In another file | Share |
|---|---|---|---|---|
| `internal/checker` | 10 | 3,222 | 384 | **11.9%** |
| `internal/gna` | 2 | 435 | 10 | 2.3% |
| `cmd/gon` | 2 | 355 | 8 | 2.3% |
| `internal/preproc`, `fmt`, `lsp`, `goenv`, `extlib`, `transpiler` | 1 each | 6–303 | 0 | 0% |
| **Total** | | **4,666** | **402** | **8.6%** |

Six of the nine packages are a single file, so a single-file check is already
sufficient for them. The checker, the largest and most important, is not.

### 12.4 Folders of independent programs

Directories with two or more `.gon` files, by the number of files that declare
their own `main()`: `testdata/valid` 4 of 5, `testdata/diagnostic-protocol/
errors` 2 of 2, `…/positions/utf8` 2 of 2, `gon-vscode/fixtures` 6 of 6;
`testdata/invalid` has 1 of 11. These are independent programs that happen to
share a folder; treating a folder as a package by default would be wrong
(P1).

Limits: one codebase (Gon itself) for §12.2 and §12.3, the counts in §12.4 are
of `main()` declarations rather than of an actual package build, and none of it
measures user projects.

## 13. Invariant Check (I1–I7)

- **I1** — upheld: the same non-flow checks, applied to more files.
- **I2** — upheld: only explicit `!` contracts; nothing inferred across files.
- **I3** — upheld: package identity, imports and types come from `go/packages`
  and `go/types`.
- **I4** — upheld: `.gna` stays authority only for external contracts (P7).
- **I5** — upheld: no runtime or code-generation change.
- **I6** — upheld: contracts are not carried across files by any assignment;
  they are looked up by declaration.
- **I7** — upheld: no new syntax, schema stays **1**.

---

**Proposed decisions (summary)**

| Decision | Proposal |
|----------|----------|
| Package mode selector | A directory / `./...` argument; a file argument is unchanged (P1, Q1: accepted) |
| Package files | Directory's `.gon` + `.go`, `.gon` shadows same-named `.go`, tests excluded (P2, Q2, Q3: accepted) |
| Type-check | `go/packages` load of the real package with per-file overlay (P3) |
| Contracts | Shared across the package's files; `!` offsets keyed per file (P4) |
| Diagnostics | Per file, deterministic order, one `--json` document (P5) |
| Type errors | Do not hide contract diagnostics; all files reported (P6, Q4: accepted) |
| Cross-package | Unchanged, `.gna` only (P7, Q5: accepted; reading `.gon` is a later RFC) |
| `./...` | Go conventions, skips `testdata` etc. (P8) |
| Schema / diagnostic codes | No change (P11) |
| Release shape | Solo v1.10; M7 moves to v1.11 (P11, Q6, Q7: accepted) |

---

## 14. Implementation Notes (v1.10)

- **API.** `checker.CheckPackage(dir, []PackageFile, resolver)` in
  `internal/checker/package.go`; it returns per-file diagnostics sorted by path
  plus the package's Go errors. The CLI side is `cmd/gon/package.go`
  (operand classification, `...` expansion, output).
- **P1.** `expandCheckOperands` classifies each operand: a `...` pattern or an
  existing directory is package mode; everything else is passed through as a
  file so the existing path keeps its own errors and wording. A directory or
  pattern with no `.gon` file is exit 2.
- **P3.** One `packages.Load(cfg, "file="+<first overlay path>)` with each
  file's clean source overlaid at `<dir>/<base>.go`; a `.gon` therefore replaces
  the same-named `.go` and is added when there is none. A file that a build
  constraint excludes is an error naming it (`… is not part of the package`).
  *(v1.15: the message is `excluded by build constraints: <files>` whatever the
  file order, and the exit code is 2, a package that cannot be loaded; a file
  the loader cannot parse is a Go error of the package instead. See
  `docs/diagnostic-protocol-v1.md` §7.)* *(v1.16: inside a module the package
  is loaded by its directory, so the go command decides membership: an
  excluded `.gon` is not checked and is not an error; only a package none of
  whose sources is in the build is, exit 2, when named explicitly.)*
  *(v1.18: outside a module, where `file=` returned a one-file package, every
  member file is named to the loader instead, membership decided by go/build;
  a multi-file package outside a module is checked as a whole.)*
  *(v1.19: a `.gon` whose `//go:build` line is malformed is a source error of
  the package, not a load failure: it is not checked, the others are, exit 1;
  a name the go tool ignores (`_x.gon`) is not part of the package. See
  `docs/diagnostic-protocol-v1.md` §7.)*
- **P4 — the refactor was smaller than the sketch.** `Check()` is now
  `collect()` then `walk()`; in package mode every file's checker shares the
  three declaration tables (maps passed by reference), so after every file has
  collected, each one sees the whole package's contracts. `!` offsets are looked
  up **by the file of the node** (`packageShared.offsets`), not from one table;
  the only place a checker reads a sibling file's offsets is walking a nested
  anonymous struct declared there, and a test pins that exact path (a mutation
  that read the wrong table was caught only by it, not by the general test
  written first). `lookupStructAST` also searches the sibling files.
- **P5.** Files are reported sorted by path; in the human output a package
  file's line is prefixed by its **path** rather than its base name, because
  `./...` can hold several `main.gon`. `--json` keeps the absolute `file` field.
- **P6.** Contract diagnostics are printed first, then `Go type error:` lines,
  then exit 1. `--json` does not carry type errors (Protocol v1 has no field for
  them), but in package mode the exit code is still 1 when the package does not
  type-check, with the `gon: Go type error:` lines on stderr and an unchanged
  Protocol v1 document (possibly empty `diagnostics`) on stdout. This was a
  review finding on the first implementation, which exited 0. Single-file
  `--json` had the same gap; it was fixed separately (#22) and Protocol v1 §7
  was clarified (#20) so that exit 1 with an empty `diagnostics` is valid. Both
  modes now agree (pinned by `TestFileAndPackageJSONAgreeOnGoTypeError`).
  Beyond type and parse errors the package's `go list` errors (for example
  mismatched package clauses) are reported as well.
- **`*_test.gon` is excluded too** (found in audit of the rebased head): the package is loaded with `Tests: false`, so a `helper_test.gon`, overlaid as `helper_test.go`, could never join it and used to fail with "is not part of the package". `gonFilesIn` now skips the name, so a directory with only `*_test.gon` files is "no .gon files" (exit 2), also for `./...`. A `*_test.gon` named as a FILE operand is checked as before.
- **M5a validation** runs per file against that file's own imports, so a package
  imported only by a sibling is not reported twice.
- **Not done, as decided:** no flag, no cross-package source contracts, no
  `transpile` / `build` directory form, no editor setting (P7, P9, P10).

