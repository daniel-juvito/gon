# Release readiness review: Part A of the explicit non-nil conversion (refresh)

**Status: closed (2026-11-22).** v1.21.0 was released and every decision is made; see
section 0. The body below is the review as it stood before the tag and is kept as a
record, not updated.  
The review itself released nothing, tagged nothing and decided nothing: the version number,
the date and the shape of the release were the owner's.  
**Baseline:** `main` at `84fb89f` (PR #76), compared with `v1.20.0`.  
**Date:** 2026-11-22  
**Supersedes** the first version of this review (branch `docs/release-readiness-part-a`,
measured on `main` `ae2ca4d`, before `&operand` became a recognised source). That branch
is not changed.  
**Downstream baselines (after the `mk()` annotations, PRs #19 and #17):** `gon-examples@92dcacc`
and `gon-vscode@9089e3d`.  
**Evidence level:** every number below was measured in this session with a `gon` built
from `main@84fb89f` unless it is marked *reported* or *not verified*.

## 0. Outcome

| | Result | Evidence |
|---|---|---|
| Release | `v1.21.0` tagged by the owner on `main` `9be4c0e` (PR #77); Release workflow `success` (tests, six binaries, `checksums.txt`, GitHub release with 7 assets) | read job by job from GitHub |
| CI of `main` for #77 | `success` | run level only |
| D1, D2, D4, D5 | done as decided (section 7) | |
| D3 | **approved by the owner as written** in the CHANGELOG `[1.21.0]` (the compatibility paragraph, not the shorter draft of section 9); no further documentation change | owner's decision, 2026-11-22 |
| After the tag, `gon` | PR #78: pin v1.21.0, nine `(!T)(x)` sites, `gon check ./...` must print `ok`; CI of `main` `success` job by job | read from GitHub |
| After the tag, `gon-examples` | PR #20: pin, `expected.txt` for the 7 examples (15 GW006), the two comments; CI of `main` `success` | run level only |
| After the tag, `gon-vscode` | PR #18: pin, `expected-fixtures.txt` for `genericgna` (2 GW006), CHANGELOG 0.1.17; CI of `main` `success` | reported by the owner, not read by me |

Still open, outside this review: Part B of the RFC, GW007 and `--strict` (undecided).

## 1. Verdict

| Question | Verdict |
|---|---|
| Is the code on `main` ready to be released? | **PASS** (technical): green CI, 86 of 86 conformance entries, no defect found by this review, no diagnostic other than GW006 changes against `v1.20.0` on the files measured |
| Is the release ready to be decided and made? | **HOLD on D3 only** (the text of the compatibility note); D1, D2, D4 and D5 were decided by the owner on 2026-11-22 (section 7). The release itself is made by the owner's tag |

Nothing found in this refresh blocks the release technically. The one open item is the text
of the note (D3), and the work that can only follow a tag (section 8).

## 2. What `main` holds since `v1.20.0`

Product changes: #69 (the `(!T)(x)` syntax and `!!T`), #70 and #73 (two fixes that stop a
`!` from being lost), #74 (checks of a conversion), #75 (GW006) and #76 (any `&operand` is
a recognised source). Others are documents, the conformance harness (#67) and the `.gna`
measurement (#72). `CHANGELOG.md` `[Unreleased]` describes them and also holds one older,
unrelated fix (the message of `gon check <dir>` on a directory with no `.gon` file); that
section sits between `[1.11.0]` and `[1.10.0]` instead of at the top.

## 3. Verified: CI of `main@84fb89f` (push)

| Workflow | Result | How it was read |
|---|---|---|
| CI, run 44975338142 | `success` | job by job: `gon-sources`, `build-and-test` (Build, Format, Format `.gon`, Vet, Test, Coverage), `test-os (macos-latest)`, `test-os (windows-latest)`, all four `success` |
| User Go, run 44975338151 | `success` | the run only; the seven jobs were not read one by one |

Also run locally on `main`: `go test ./...` passes in every package; the conformance
manifest has **86 entries, all enabled, none pending**.

The User Go workflow was read job by job: all seven jobs (Go 1.21.13 to 1.27.0) `success`.
The downstream repositories after the merge of the `mk()` annotations, read job by job: CI of
`gon-examples@92dcacc` (job `examples`, with the step "Check the examples against expected.txt")
and of `gon-vscode@9089e3d` (jobs `Fixtures against gon` and `Extension tests`) are all `success`.

## 4. Measured: warnings on the three repositories

`gon` built from `main@84fb89f`. The control is `gon` v1.20.0, the pin of the three
repositories: `gon-examples/scripts/check-examples.sh` (20 examples) and
`gon-vscode/scripts/check-fixtures.sh` (11 fixtures) both pass with it, 0 failures.

| Repository | v1.20.0 | `main@84fb89f` | Earlier review (`ae2ca4d`) |
|---|---|---|---|
| `gon` production, `gon check ./...` | `ok` | **9 GW006**, no other diagnostic | 10 |
| `gon` `testdata/`, file mode | 0 | 11 GW006 | 14 |
| `gon-examples@92dcacc`, 20 examples | 20 of 20 pass | **7 of 20 differ**, **14 GW006** | 11 of 20, 35 (7 of 20, 26 before the `mk()` annotations) |
| `gon-vscode@9089e3d`, 11 fixtures | 11 of 11 pass | **1 of 11 differs** (`genericgna`), **2 GW006** | 3 of 11, 7 (2 of 11, 4 before the annotation) |

In all cases the only difference against v1.20.0 is added GW006: no exit code, no other
diagnostic code and no Protocol v1 field changes (earlier comparison of 61 runs, repeated
here only through the example and fixture scripts).

### Where the warnings came from, and what the `mk()` annotations removed

The table is the classification of the 26 + 4 warnings that `main` gave **before** the annotations
of PRs #19 and #17. Those PRs removed the first row (12 in `gon-examples`, 2 in `gon-vscode`); the
measurements above are after them (14 and 2), re-measured on the merged commits.

| Cause | gon-examples | gon-vscode | What removes it |
|---|---|---|---|
| a plain function `mk()` that always returns a non-nil value, used in a `!T` slot | 12 (`element-contracts` 11, `generics` 1) | 2 (`elementcontracts`) | **done**: `mk() !*T` (#19, #17) |
| a local built from a literal (`db := &store.DB{}`) then used in a `!` field | 8 (`ecosystem-contracts` 6, `generic-gna` 2) | 2 (`genericgna`) | Part B (binding by inference), or write the literal in place, or `(!T)(x)` |
| a parameter or variable that the author knows is non-nil | 4 (`field-contracts` 2, `type-coverage` 1, `generics` 1) | 0 | `(!T)(x)`, the claim |
| `maybe()`, which may return nil on purpose | 1 (`element-contracts`) | 0 | `(!T)(x)` if the author means it |
| `get()` in `flow-ok`, whose comment says "allowed in v1.0 - not a diagnostic" | 1 | 0 | the example's text must change |
| **total** | **26** | **4** | |

The rows are classified by the expression and by the declaration of the function in the
example; the classification was read by eye on the 30 lines. The first row, the cheapest change
available, is done: 14 of 30 removed. **What is left (16): 10 locals built from a literal (Part B),
4 claims of the author, `maybe()` and `get()`.**

### The `&x` finding of the first review is closed

The first version of this review found that 9 of 35 warnings in `gon-examples` were `&x`
(`greet(&app)` in the first example of the README). PR #76 made any `&operand` a recognised
source; `hello`, `cli`, `return-value` and `construction` are clean again (13 examples now
match their `expected.txt`). The narrow rule (`&identifier`) and the wide rule give the
same result on every file of the three repositories, so the widening has no measured
effect there; it rests on the tests and on the argument recorded in the RFC.

## 5. Reconciliation of the earlier figures

| Figure | Where it was said | Now |
|---|---|---|
| "10 conversions to migrate Gon's own sources" | RFC 10.7 before #76; the dogfood branch | **9**; `&cp` is recognised |
| "10 GW006 in `gon` production" | the first review, RFC 2.1 | **9** |
| "gon-examples 10 and 4" (instrument, five modules) | RFC 2.1 before #71 | not comparable: the instrument could not load 14 demos; the real checker gives 26 on all 20 |
| "35 warnings, 11 of 20 examples differ" | the first review | 26 and 7 |
| "RFC 11 and 12 (Part A+B)" | RFC before #76 | 10 and 11 |

The RFC (2.1, 5.1, 10.7 and the dogfood criteria) was brought to the figures of
`main` in #76. The figures of the instrument and of the real checker should not be mixed:
the instrument is scratch code and is UNAUDITED as a measurement of the checker.

## 6. Compatibility of the release

| Change | Effect on existing code | Breaking? |
|---|---|---|
| GW006 | new warnings (exit 0) on flows that were silent | **possibly**: a CI that fails on any warning, or compares the number of diagnostics, changes behaviour |
| `! *T` enforced (#73) | a contract that was silently lost is now reported (GN001, GN002: errors, exit 1) | **possibly, an error that was accepted**; none in the three repositories |
| `gon fmt` error on an unmatched mark (#70) | exit 1 and the file untouched, where it used to exit 0 and drop the `!` | behaviour change in a corrupting case |
| `(!T)(x)`, `!!T` | were parse or Go type errors; now accepted | no (new syntax) |
| exit codes, `schemaVersion`, JSON fields | unchanged | no |

Part A is a "potentially breaking semantic RFC" under R1 of the roadmap, so it should be
released alone. The older `[Unreleased]` message fix is non-breaking (R2) and may travel
with it.

## 7. Decisions

Decided by the owner on 2026-11-22 (answers to D1, D2, D4 and D5 recorded here as given; the
recommendation of this review was followed in each).

| | Decision | Status |
|---|---|---|
| **D1** | **v1.21.0, a minor release of its own** (R1: Part A is a potentially breaking semantic change). The older unrelated fix (the message of `gon check <dir>`) travels with it (R2, non-breaking). The date is the day the tag is made; this review does not set it | **decided** |
| **D2** | **Release Part A first.** Part B stays OPEN and is released on its own later (R1). Consequence, accepted: `x := &T{}; f(x)` into a `!` slot stays a warning under Part A (10 of the 16 warnings left in the examples) | **decided** |
| **D3** | The text of the compatibility note (section 9) | **open**: a draft is in section 9 for the owner to edit or approve |
| **D4** | The warnings left in the examples are handled **after the tag, in the PR that bumps the pin**: the two comments (`maybe()` in `element-contracts`, `get()` in `flow-ok`), `expected.txt` for the 7 examples and `expected-fixtures.txt` for `genericgna`. Before the tag nothing more changes, because the new syntax cannot be read by gon v1.20.0 | **decided** |
| **D5** | **`gon` first, then the two repositories**, one PR each, in this order (section 8) | **decided** |

What is *not* decided and stays with the owner: the day of the tag, and the approval of the text of
the note (D3). Nothing in this review is a release, a tag or a version change.

## 8. How a release is made here, and what follows the tag

- A tag `v*` runs `.github/workflows/release.yml`: `go test ./...`, six binaries (linux,
  darwin, windows, amd64, arm64) with `CGO_ENABLED=0`, `checksums.txt`, and the GitHub
  release with generated notes. I do not tag.
- Places that name the version: `cmd/gon/main.gon` and `main.go` (`gon version 1.20.0`), the
  `go install` line and a new row in the version table of `README.md`, `CHANGELOG.md`
  (`[Unreleased]` becomes the release, with a date), `docs/roadmap-v1.3-plus.md`,
  `docs/dogfood.md`, and the `(on main, not yet released)` markers in `README.md`.
- After the tag, in the order of D5, each as its own PR: `gon`: pin `GON_VERSION` in
  `.github/workflows/ci.yml`, then the migration of the **nine** sites (the dogfood branch
  `chore/dogfood-candidate-gate` was refreshed on `main@84fb89f`: nine conversions, each shown to
  be needed, no `&cp`) and a permanent `gon check ./...` gate; `gon-examples`: the pin,
  `expected.txt` for the 7 examples that differ and the two comments (D4); `gon-vscode`: the pin
  and `expected-fixtures.txt` for the 1 fixture (`genericgna`). The `gon-sources` job of the migration PR needs
  the new pin to be green, because v1.20.0 cannot read `(!T)(x)`.

## 9. Draft of the compatibility note for the CHANGELOG (for the owner to edit)

> **New warning GW006.** `gon check` now warns when a value that Gon cannot see to be
> non-nil flows into a `!T` slot of a pointer, slice, map, channel or function kind. It is
> a warning: the exit code is unchanged. Silence it where you know the value is non-nil: write
> `(!T)(x)`, declare the function that returns it `!T` (its `return` is then checked and its
> callers are silent), or use a construction form (`&x`, `&T{}`, `new`, `make`, a literal). An
> ordinary interface value into `!I` is still an error (GN001). A `!` followed by a space
> (`var x ! *T`) is now enforced, where it was silently ignored.

## 10. Risks and limits of this review

- Only the three repositories are measured; for users outside them the amount of new GW006
  is unknown. The examples are small and written to show contracts, so they may overstate
  or understate real code.
- The `.gna` measurement and its instrument are UNAUDITED as a measurement of the checker.
  The figures of section 4 come from the real checker.
- The classification of section 4 was read by eye on 30 lines.
- CI of `main` was read job by job for the CI workflow, and only at run level for User Go.
- Not examined: the VS Code extension's packaging and version, any download or install script
  outside the three repositories, and the behaviour of the release workflow (it was read, not
  run).
