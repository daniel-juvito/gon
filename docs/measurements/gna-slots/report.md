# Measurement of `.gna` contract slots before PR 2b (plan section 5)

Status: **new measurement, UNAUDITED** (kept on a branch for audit; not merged, no PR). The measured sources
were not changed. Basis: `gon` `main` 97bba38, `gon-examples` and `gon-vscode` as checked out in this session
(`main`), Gon built from `main` for the control runs. The decisions of the RFC (Part A accepted, GW006 also on
`.gna` slots, Q1 of the plan) are not reopened by this report; the plan says an unexplained difference
puts the GW006 implementation on HOLD.

## 1. What was measured

The instrument is `g9measure` (scratch code, kept here as `g9measure-main.go.txt`; it is not part of the build), the v2 tool `g8measure` of
the M0 re-measurement plus the `.gna` model required by plan section 5:

| Added in v3 | How |
|---|---|
| `.gna` signatures of external functions and methods | read with the repository's own `internal/gna` registry (`LoadDir` on `<module>/annotations`, nested directories included); looked up by import path, function or `Type.Method`; a package that has `.gon` sources keeps its source signature, as the checker does (GW005 for a stale `.gna`) |
| `.gna` results as recognised sources | `classifyCall` uses the `.gna` result flag, so `r := store.Open()` binds under today's rules |
| `.gna` parameters as slots | arguments are flows into a `!T` parameter |
| `.gna` `types:` fields as slots and sources | `store.Conn{DB: x}`, `c.Log` (source), `c.Log == nil` (GW001) |
| explicit instantiation `f[T](...)` | the callee is found through the index expression (see the control below) |
| `G9_NOGNA=1` | empty registry, to isolate what `.gna` changes |

Eight `.gna` files exist in the three repositories; five modules use one that is not shadowed by `.gon`
sources: `gon-examples/ecosystem-contracts`, `gon-examples/generic-gna`, `gon-vscode/fixtures/ecosystem`,
`gon-vscode/fixtures/genericgna` and `gon`'s own `annotations/` (`os.gna`, `io.gna`, `extlib.gna`).
`source-contracts` and `sourcecontracts` carry a stale `.gna` that `.gon` sources shadow (GW005).

Rules: mode 0 = Part A (binding as Gon v1.20.0 does it, plus `(!T)(x)`, which occurs nowhere); mode 1 =
Part A + Part B (the binding of locals by inference, `append` and slice rules).

## 2. Controls

**Predicted GW001 under today's rules (mode 0) against the real `gon check --json` (Gon from `main`).** This
is the check that could not be made in the M0 report ("could not be assessed in four" modules).

| Module | Predicted | Actual |
|---|---|---|
| gon-examples/ecosystem-contracts | 3 | 3 |
| gon-examples/generic-gna | 2 | 2 |
| gon-vscode/fixtures/ecosystem | 1 | 1 |
| gon-vscode/fixtures/genericgna | 2 | 2 |
| gon-vscode/fixtures/sourcecontracts | 1 | 1 |

The first run of v3 predicted 1 against 2 on the two generic modules: explicit instantiation
(`cache.Must[int](&x)`) was not resolved, which the M0 report listed as a limit. The callee is now found through
the index expression and the prediction is 2 of 2. The ecosystem modules needed the `.gna` field in the
nil-comparison path (`c.Log == nil`); added, 3 of 3.

**Consistency with the earlier numbers.** On `gon` (production and fixtures) mode 0 gives 19 GW006 sites and
mode 1 gives 20, identical with and without `.gna`: 10 and 11 in production plus 9 in the fixtures, as in the
RFC. The `.gna` contracts of `os`, `io` and `extlib` therefore change nothing for `gon` itself.

Not a control of GW006: no checker implements it, so these controls validate the instrument and the
GW001 prediction under the old rules only.

## 3. Result

GW006 sites, by module, with and without the `.gna` model:

| Module | Part A, with `.gna` | Part A, without | Part A+B, with | Part A+B, without |
|---|---|---|---|---|
| gon (production + fixtures) | 19 | 19 | 20 | 20 |
| gon-examples/ecosystem-contracts | **6** | 0 | **0** | 0 |
| gon-examples/generic-gna | **4** | 0 | **2** | 0 |
| gon-vscode/fixtures/genericgna | **4** | 0 | **2** | 0 |
| gon-vscode/fixtures/ecosystem | 0 | 0 | 0 | 0 |
| gon-examples/package-check, source-contracts, transpile-dir; gon-vscode/fixtures/sourcecontracts | 0 | 0 | 0 | 0 |
| 14 single-file modules of gon-examples (see "Coverage") | **not measured** | | | |

New sites that exist only because `.gna` is modelled (every one is a flow into a `.gna` slot):

| Site | Slot | Value | Part A | Part A+B | Classification |
|---|---|---|---|---|---|
| ecosystem-contracts `demo.gon:20, 25, 28 (x2), 31 (x2)` | `!*DB`, `!*Logger` fields of `store.Conn` | local `db`, `lg` declared by `db := &store.DB{}` | GW006 | silent | local bound by Part B only (`&literal`); in Part A the value is a plain identifier |
| generic-gna `demo.gon:23, 24` | `!*Logger` field of `cache.Cache` | local `logger := &cache.Logger{}` | GW006 | silent | same, Part B binding |
| generic-gna `demo.gon:11, 14` (copied in gon-vscode `genericgna.gon`) | `!*V` parameter of `cache.Must` | `&x`, address of a local variable | GW006 | GW006 | `&x`, which the RFC leaves out of scope on purpose (counted as `&cp`); always non-nil, a known false-positive class, not a new one |

What `.gna` also changes, in the other direction: the flows of `ecosystem-contracts` go from 1 to 11, with 6 silent
and 4 interface destinations (D3a/D3b/D3d, unchanged, not GW006). `store.Open()` and `store.Connect()` are
recognised sources, so the one "false positive: `.gna`-annotated `store.Open()`" of the RFC table (section 2.1) is
gone; it was an effect of the instrument not reading `.gna`.

## 4. Comparison with the RFC prediction

- **The rule applies to `.gna` slots as predicted.** The RFC (section 6.6 and Q1 of the plan) says GW006 applies to
  a parameter, result or field slot, wherever it is declared. Every new site is a flow of an ordinary value into
  such a slot; there is no site where the instrument and the RFC disagree about which flows are warnings.
- **Part A alone warns where Part A+B is silent, exactly in the cases the RFC states.** The 6 + 2 sites of
  Part A that are silent in Part A+B are locals built from a literal and passed on, which only Part B binds.
  This is the cost of accepting Part A without Part B for users of `.gna` field contracts, and it is the same
  trade-off as in `gon` itself (nine guard and constructor sites plus `&cp`).
- **Difference with the RFC migration table (section 2.1):** the table says "gon-examples: 0 genuine" and counts
  no site for the generic `.gna`. With `.gna` modelled there are 10 flows in `gon-examples` in Part A (6 in
  `ecosystem-contracts`, 4 in `generic-gna`) and 2 in Part A+B (both in `generic-gna`, both `&x`), in a file that
  exists to demonstrate GW001 and not to be warning-free. The difference is **explained**: the earlier instrument did not read `.gna` ("not modelled"
  in the RFC itself), and the M0 report listed it as a limit. It is not an unexplained difference in the sense of
  plan section 5, and it does not by itself put GW006 on HOLD. It does change two numbers the RFC prints
  (gon-examples and gon-vscode fixtures) and the fixtures of PR 2b: the two examples need the rule applied to them
  or an explicit conversion, decided when PR 2b is authorised.
- **No genuine bug and no false positive other than `&x`.** I found no site where the warning is wrong for a
  reason other than the documented `&x` limit.

## 5. Consequences for PR 2b (to be decided by the owner, not by this report)

1. The `.gna` fixtures of `gon-examples` and `gon-vscode` (`ecosystem-contracts`, `generic-gna`, `genericgna`)
   are affected: 14 flows in Part A and 4 in Part A+B across the two repositories if the copy is counted, 10 and 2
   if it is counted once (`gon-vscode/fixtures/genericgna` is a copy of `generic-gna`). Their `expected.txt` and the
   editor fixtures would change when GW006 exists; they are not touched here.
2. `cache.Must(&x)` is the visible cost of leaving `&x` out of scope. The RFC already records `&x` as a separate
   question; this is the first `.gna`-slot evidence for it. I do not propose to reopen it.
3. The conformance harness has no `.gna` slot fixture yet (listed under "Not covered" in the PR 0 README).
   PR 2b would add them: a `.gna` parameter, a `.gna` field and a `.gna` result as a source.

## 6. Limits

- Only the three repositories at the commits named above. `.gna` of external packages that users write, generic
  declarations beyond the two `.gna` generic modules, `range`, channel sends and closures are not modelled.
- Receiver contracts (`receivers:` in `.gna`) are not modelled; no `.gna` file of the three repositories uses
  a receiver claim as a slot.
- The instrument is scratch code and is not part of any repository. Its numbers hold for the covered
  repositories only and do not validate GW006 itself, which no checker implements.
- Raw outputs: `raw/*.txt` (per module, mode and `.gna` on/off; `m0` = Part A, `m1` = Part A+B, `without` = `.gna` off).

## Unit of the counts

Sites are counted as **flows** (one per value flowing into a slot), not as distinct lines: in
`ecosystem-contracts`, lines 28 and 31 each hold two flows (two `.gna` fields in one literal), so Part A has 6
flows on 4 lines. `gon-vscode/fixtures/genericgna` is a copy of `gon-examples/generic-gna` (same flows, a
different file name), so the unique count for the two repositories is 10 flows in Part A and 2 in Part A+B.

## Reproduction of the controls

`actual/*.check.json` are the outputs of `gon check --json .` (Gon built from `main` 97bba38, run in the module
directory) for the five modules of the control table. The GW001 count of each file is the "Actual" column
(3, 2, 1, 2, 1); the "Predicted" column is the line `nil-compare => GW001 | exists today` of
`raw/<module>.m0.with.txt`. Generic instantiation is exercised by `generic-gna` and `genericgna`
(`cache.Must[int](&x)`), whose prediction is 2 only with the instantiation fix described above.

## Notes on the instrument

- Loading of `.gna` files: the first version reported an error from the top-level `LoadDir` and ignored the
  errors of nested directories, and it ignored the result of `filepath.Walk`. Now a `LoadDir` error in any
  directory under `annotations/` and an error of the walk itself (a directory that cannot be read) stop the run
  with exit 1, so a partial registry cannot give partial counts. Checked with a nested malformed `.gna`
  (`annotations/sub/deep/bad.gna`): exit 1 with the file named. The walk-error branch itself was **not
  triggered** (the session runs as root and can read every directory); it rests on the code, not on a test.
- Source discovery: the first versions ignored the error of the walk over the module (`return nil` in the callback,
  `filepath.Walk` result unused) and the error of reading a `.gon` file (`src, _ := os.ReadFile`). Both now stop
  the run with exit 1. Checked with a dangling `dangling.gon` symlink (exit 1, file named); the walk-error branch
  itself was not triggered, for the same reason as above (root can read everything).
- Classification of `raw/`: recounted by the first line of each file. 36 files start with the result header
  (`# <module> ...`), 56 do not, and all 56 contain `cannot find main module`.
- All 92 outputs were produced again with the strict version of every check above. They are byte-identical to the ones in `raw/`;
  `raw.sha256` holds the SHA-256 of each, so `cd raw && sha256sum -c ../raw.sha256` checks the files, and the
  regenerated set was compared with the same list (0 differences).
- A field contract is matched by the name of the field on the named type (`Types.<T>.Fields`). Fields reached
  through an embedded struct or a type alias are not resolved safely by this approach. I did not check whether any of
  the measured `.gna` fields is reached that way, so this is a limit of the instrument and one more reason the
  counts hold for the covered modules only.

## Coverage (corrected)

Twenty-three directories were given to the instrument. **Nine were measured**: `gon`; the five modules of
`gon-examples` that have a `go.mod` (`ecosystem-contracts`, `generic-gna`, `package-check`, `source-contracts`,
`transpile-dir`); and the three subdirectories of `gon-vscode/fixtures` (`ecosystem`, `genericgna`,
`sourcecontracts`). **Fourteen were not measured**: the single-file demos of `gon-examples` (`cli`,
`construction`, `element-contracts`, `field-contracts`, `flow-ok`, `generics`, `hello`, `interface-contracts`,
`method-contracts`, `reject-nil`, `return-contracts`, `return-value`, `struct-fields`, `type-coverage`) have no
`go.mod`, so the package loader fails (`cannot find main module`) and the instrument prints that error instead
of a result. **56 of the 92 files in `raw/` are that error** (14 modules x 4 runs); the other 36 are results.
An earlier version of this report listed these modules under "every other module: 0", which was wrong: they
produced no number. None of the 14 has a `.gna` file or an `annotations/` directory, so the `.gna` model cannot
change them, and the five RFC "modules" of `gon-examples` are the five that were measured. The loose `.gon`
files directly under `gon-vscode/fixtures` were not given to the instrument either. The conclusion of section
4 (no unexplained difference for the `.gna` slots) concerns the nine measured directories and the five
that carry a `.gna` file; it says nothing about flows in the unmeasured demos, which are outside plan section 5.
