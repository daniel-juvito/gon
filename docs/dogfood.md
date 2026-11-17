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
| `internal/lsp` | 1 | yes (byte-identical transpile) | none yet |
| `internal/preproc` | 1 | yes (byte-identical transpile) | none yet |
| `internal/gna` | 2 | yes (byte-identical transpile; first multi-file package) | `NewRegistry` → `!*Registry`, `DefaultChain` → `!*Chain` (local only, see below) |
| `cmd/gon` | 4 | yes (byte-identical transpile) | none yet |
| `internal/checker` | 12 | yes (byte-identical transpile) | none yet |

## Annotations (D5) and what they reach

Owner decision (2026-11-17): **A + C**. A: annotate `internal/gna`'s two
results (`NewRegistry`, `DefaultChain`) now. C: annotate no further
**exported** API until an RFC for cross-package contracts read from a
module-local package's `.gon` sources is written and accepted. Rejected (B):
copying the same contracts into an `annotations/…/internal/gna.gna`, because
nothing would keep the two copies' `!` flags equal.

Why C: with the M8 model, a package that **imports** a module-local package
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
