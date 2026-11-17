# Writing Gon in Gon (dogfood)

Gon's own sources are being converted to `.gon`, one package at a time. This
page records the owner's decisions (2026-11-17) and the procedure.

## Decisions

| # | Decision |
|---|----------|
| D1 | Calls into `os` and `io` are covered by the repository's own `annotations/os.gna` and `annotations/io.gna` (the documented `<module-root>/annotations/` convention), completed with every symbol Gon's sources use, all **ordinary** (no `!`). The files stay where they are; they are also the fixtures of several tests. |
| D2 | Each converted package commits **both** the `.gon` sources and the `.go` that `gon transpile` emits (`go install …@vX` needs the `.go`). CI proves they agree. |
| D3 | CI checks and transpiles the sources with a **pinned released** `gon` (`go install …@<GON_VERSION>`), never with the checker built from the commit under test. The pin is bumped on purpose. |
| D4 | A preparation change first (D1 and the CI gate, with no `.gon` yet), then **one PR per package**, starting with the smallest single-file package; `internal/checker` last. |
| D5 | A conversion PR only renames `.go` → `.gon` and proves the transpile is byte-identical: **no `!`**. Contracts are added in later, separate PRs per package, each listing the diagnostics it introduces. |

## Acceptance criteria

- **Conversion PR:** `gon transpile` reproduces the committed `.go` byte for byte;
  no new semantic annotation.
- **Annotation PR:** every new diagnostic is reviewable as an intended contract
  change.

## The gate

`scripts/check-gon-sources.sh` (CI job `gon-sources`) finds the packages under
`cmd/`, `internal/` and `transpiler/` that have `.gon` sources, runs
`gon transpile` on them (the directory form checks each package as a whole and
writes nothing for one that does not pass), and then fails if any `.go` differs
from the commit or is untracked. With no `.gon` yet it passes with "nothing to
check". Measured on a scratch clone: it accepts a correct conversion and
rejects a `.gon` edited without regenerating its `.go`, a `.go` edited by hand,
a converted package whose `.go` was never committed, and a `.gon` with a
contract error.

## Converting a package

```bash
cd <package dir>
for f in *.go; do case "$f" in *_test.go) ;; *) cp "$f" "${f%.go}.gon" ;; esac; done
cd -
gon transpile ./<package dir>         # must print transpiled: … and change nothing
git status --porcelain -- '*.go'      # must be empty
scripts/check-gon-sources.sh
```

Tests stay `.go` (`*_test.gon` is not part of the directory form).

## Progress

| Package | Files | Converted | `!` contracts |
|---------|-------|-----------|---------------|
| `transpiler` | 1 | yes (byte-identical transpile) | none yet |
| `internal/goenv` | 1 | yes (byte-identical transpile) | none yet |
| `internal/fmt` | 1 | yes (byte-identical transpile) | none yet |
| `internal/lsp` | 1 | yes (byte-identical transpile) | `New` → `!*Server` |
| `internal/preproc` | 1 | yes (byte-identical transpile) | `Process` → `!*Result` (#40) |
| `internal/gna` | 2 | yes (byte-identical transpile; first multi-file package) | `NewRegistry` → `!*Registry`, `DefaultChain` → `!*Chain` (package-local before v1.14; importers also see them since M10) |
| `cmd/gon` | 4 | yes (byte-identical transpile) | none yet |
| `internal/checker` | 12 | yes (byte-identical transpile) | `withPath` → `!*ContractTrace`, `cloneGnaFile(!*gna.File) !*gna.File` (P1); `Checker.fset` → `!*token.FileSet` (P2) |

## Annotations (D5) and what they reach

Owner decision (2026-11-17): **A + C**. A: annotate `internal/gna`'s two
results (`NewRegistry`, `DefaultChain`) now. C: annotate no further
**exported** API until an RFC for cross-package contracts read from a
module-local package's `.gon` sources is written and accepted. Rejected (B):
copying the same contracts into an `annotations/…/internal/gna.gna`, because
nothing would keep the two copies' `!` flags equal.

**C is implemented in v1.14 (M10, `docs/rfc-source-contracts.md`):** an
importer now reads a main-module package's contracts from its `.gon` sources,
so `gna.DefaultChain`'s `!*Chain` reaches `cmd/gon` and `internal/lsp`. The
gate's pinned `gon` is **v1.14.0** (bumped 2026-11-17; with it, `gon check
./...` on this repository is `ok` and every `.go` still matches its `.gon`).
That bump is what let the annotations of the two remaining exported
candidates go ahead; both are done (below).

**All four syntactic candidates are now annotated:** `gna.NewRegistry` →
`!*Registry` and `gna.DefaultChain` → `!*Chain` (#36), `preproc.Process` →
`!*Result` (#40; its importer `internal/fmt` had an unreachable nil check,
removed), and `lsp.New` → `!*Server` (no importer compares it with nil, so no
diagnostic changed). Further annotations (invariant fields, parameters that are
never nil) need the author's judgement rather than a syntactic proof, and are
proposed package by package with the same measurements before any code.

**Judgement-based annotations, `internal/checker` (Proposal A, owner decision
2026-11-17: P1–P3 approved, P4 deferred).** Each group was measured before any
code: every constructor and composite literal (production and tests), every
assignment and nil comparison, the delta of `gon check ./...`, and a deliberate
violation proving the contract is enforced. What a contract guarantees is
static: Gon rejects a `nil` returned, passed or stored in its place in the
`.gon` sources. That the values are non-nil at run time rests on the
construction invariants, which a Go test locks for each group; the contract
does not make a value that comes from outside those paths non-nil.

| Group | Contracts | `gon check ./...` delta | Violation probe | Invariant test |
|-------|-----------|-------------------------|-----------------|----------------|
| P1 | `(*ContractTrace).withPath` → `!*ContractTrace`; `cloneGnaFile(f !*gna.File) !*gna.File` (its only caller skips a nil file) | none | `return nil` / `cloneGnaFile(nil)` → GN001 | `TestWithPathNeverReturnsNil` (nil and non-nil receiver), `TestCloneGnaFileNeverReturnsNil` |
| P2 | `Checker.fset` → `!*token.FileSet` (built by `New` and `CheckPackage` with `token.NewFileSet`, replaced only by `typeCheckFallback` with another `NewFileSet`) | none | a `Checker` literal without `fset` → GN002; `c.fset = nil` → GN001 | `TestNewFillsFset` (`New`, `NewWithAnnotations`, in and outside a module), `TestTypeCheckFallbackKeepsFset`, `TestCheckPackageCheckersHaveFset` (positions of each file's diagnostic; `CheckPackage`'s checkers are not exposed) |

P4 (`Checker.file`) is deferred: production never leaves it nil, but the
annotation makes three `c.file == nil` guards GW001, and one of them is the
behaviour `TestTypeCheckFallbackWithoutSourceIsNoOp` locks.

Why C (as measured before M10): with the M8 model, a package that **imports** a module-local package
reads that package's contracts from `.gna`, never from its `.gon` sources.
Measured: with `!*Chain` on `DefaultChain`, `c := gna.DefaultChain(…); c == nil`
is **not** a GW001 in `cmd/gon` (the importer), and **is** one inside package
`gna`. So these annotations protect the **function body** (a `return nil` there
becomes GN001) and do **not** protect callers in other packages. All four
syntactic candidates (`gna.DefaultChain`, `gna.NewRegistry`, `preproc.Process`,
`lsp.New`) are called only from other packages.

**Conversion complete:** every production package is written in Gon; the
`.go` files are what the pinned `gon` emits for them, checked by CI. The next
step is D5's annotation PRs, one per package.

`internal/extlib` is a test fixture that stands for an *external* Go library in
the `.gna` tests; converting it would blur that role, so it stays
`.go` (confirmed by the owner, 2026-11-17).
