# Implementation plan: explicit non-nil conversion `(!T)(x)`, Part A

**Status:** Proposal (OPEN / UNAUDITED). A plan only: it contains no code and does
not authorise implementation, a release, or a schedule.  
**Basis:** `docs/rfc-explicit-nonnil-conversion.md` (Part A accepted as a design
decision on 2026-11-21). Part B, G1–G6, `--strict`, GW007, `&x` and the release
shape are outside this plan and unchanged.  
**Date:** 2026-11-21

## 0. Decisions fixed by the plan audit

| Question | Decision |
|----------|----------|
| Q1: slots declared by `.gna` | GW006 also applies to `.gna` contract slots; **measure first, before PR 2b** (§5). If the result does not match the RFC, GW006 is on HOLD until the difference is explained |
| Q2: nested parentheses | Supported to any valid depth; the canonical form is what `go/format` produces (one layer), see WP2 |
| Q3: PR 2a and 2b | Separate PRs; **a release is not automatic**, it is a separate decision once both are integrated |
| Q4: `gon check ./...` | A permanent CI gate (order in WP7) |
| Q5: code | GW006, a warning |
| Q6: tests first | PR 0, **with `main` staying green** (strategy in §4) |
| Q7: dogfood before the release | Decided: **two gates**. A pre-release gate on a development branch with a locally built candidate, then a post-release migration gate (§4, WP7) |
| Q8: canonical form of nested parentheses | Decided: `go/format` is the authority and may reduce redundant parentheses; `reinsertBangs` tolerates only parentheses it can prove were removed as redundant wrappers (WP2) |

## 1. Scope of Part A (RFC §5.1)

In scope: the syntax `(!T)(x)`; the semantics of the conversion (recognised
source, `(!T)(nil)` is GN001, a non-nilable target is GN003, Go decides validity);
the contract identity rule of RFC §6.4; GW006 for pointer, slice, map, channel and
function in every slot (a variable initialiser of type `!T`, an assignment to a `!`
slot, an argument, a `return`, a struct field in a composite literal, an element of
`[]!T{}`), including `.gna` slots; `!!T` in type positions; the amendments to I6 and
D6b; the Part A dogfood criterion (10 sites).

Out of scope: Part B (binding of `:=` and `var`, `append`, slice expressions as
sources, `x = nil` as GN001 on a bound local, I2); fixing the v1.20.0 rebinding of
existing identifiers in `:=` (an accepted limit, it **must not be fixed silently**);
`--strict`; GW007; `&x`; inference from a body; runtime checks; flow analysis; the
release shape.

Recognised sources in Part A (RFC §6.6): an identifier declared `!T` or bound by
the rules that exist today (a call result declared `!T`), a call whose first result
is `!T`, a selector of a `!` field, the construction forms for a direct flow (a
composite literal, `&literal`, `new`, `make`, a function literal), and `(!T)(x)`.
Parentheses do not change recognition.

## 2. Conformance matrix as the oracle

The behaviour is defined by this matrix (PR 0), not by the implementation. Every
cell has a fixture with the expected diagnostic.

| Dimension | Minimum cases |
|-----------|---------------|
| Target | pointer, slice, map, channel, function (GW006); interface (D3a/D3b/D3d, unchanged); type parameter (D3b unchanged) |
| Non-nilable target | `(!int)(x)`, `(!S)(x)` with a struct: GN003 |
| Operand | the `nil` literal (GN001), an ordinary value, a recognised source |
| Type identity | identical (silent), not identical but Go-assignable (GW006), not Go-assignable (a Go error, with no GW006 in its place) |
| Interface | a concrete operand (D3a, silent), an ordinary interface value (GN001), `(!I)(rc)` (silent when `(I)(rc)` is valid) |
| Explicit conversion | valid and invalid according to Go; `(!U)(x)` is silent only if `(U)(x)` is valid |
| Source nilability | an ordinary `T`, a declared `!T`, a call result `!T`, a `!` field |
| Slot declaration | local, parameter, return (multi-return), field (keyed and unkeyed), element, method argument, cross-package module-local, `.gna` |
| Expression | nested, selector, multi-value |
| Look-alikes that are not conversions | `adder(!flag)(3)` (a call whose result is called, valid Go with the token shape `(!flag)(3)`), `(!a) && b`, `x.(!T)` (D6a), `!` inside comments and strings, unbalanced or unrelated parentheses; each keeps its old meaning and old clean source |
| Part B must not leak | `append`, slice expressions and locals bound by inference stay **not** sources (GW006); `(!T)(x)` binds no local |
| Contract | no implicit propagation across types; no runtime check |

A traceability table, `RFC clause → fixture → checker site → diagnostic`, is kept in
the repository as a test file or manifest and updated by every PR. A conformance
summary per PR (active and pending fixtures per clause) goes in the PR description.

## 3. Work by component

### WP1. `internal/preproc`
- `(!T)(x)`: a `!` after a **grouping** `(` whose closing `)`, after skipping wrapping
  `)` that balance the `(` immediately before it, is followed by `(`. Parentheses
  are balanced with a counter, not a regular expression.
  **Precondition found while auditing the plan:** the `(` must be a grouping
  parenthesis, that is, the previous significant token must not end an operand
  (an identifier, `)`, `]`, `}`, a literal). Otherwise the `(` opens a call's
  argument list and the shape is not a conversion: `adder(!flag)(3)`, where `adder`
  returns a function, is valid Go whose token shape is `(!flag)(3)` (verified by
  compiling it). The safety argument of RFC §6.1 (`(!a)(b)` with a `bool` `a` is not
  valid Go) covers only the grouping case; RFC §6.1 should state the precondition
  (see §9).
- A run of `!` in a type position: a `!` after a `!` that is a type modifier is also
  a modifier (one offset); `!!flag` in an expression stays intact.
- Fixtures (PR 0): `(!T)(x)` and nested forms (one, two, three layers, with
  spaces); `!` in a type position versus an expression position; separation from a
  type assertion `x.(!T)` (D6a); complex types (`![]T`, `!map[K]V`, `!*T`,
  `!func(...)`, `!chan T`, qualified, generic); comments and strings containing the
  pattern; **non-conversion and false-positive fixtures**: a call whose result is
  called (`adder(!flag)(3)`), `(!a) && b`, an unbalanced parenthesis, `x.(!T)`, an
  index or selector before the `(`, each required to keep its old meaning and its old
  clean source and offsets; a differential suite with three criteria (identical / expected output /
  looks similar but keeps its old meaning) over every `.gon` and `.go` of the three
  repositories plus synthetic Go-valid forms.
- High risk: WP1 does not pass because one fixture passes.

### WP2. `internal/fmt` and `transpiler`
- Offset based: `!!T` becomes one `!` automatically; a conversion through the type
  offset inside the parentheses.
- **Important finding:** `go/format` **removes redundant parentheses** around a
  conversion target (measured: `((*A))(x)` and `( (*A) )(x)` become `(*A)(x)`).
  `reinsertBangs` aligns the clean and formatted token streams in lockstep and
  tolerates only a comma difference; without handling, a removed parenthesis makes
  the alignment fail ("token streams diverge") and the file is left unformatted.
  Plan: tolerate the `(` and `)` that `go/format` removes on the clean side (like
  the comma), under an **acceptance rule**: only a run of `(` immediately before a
  marked type, together with the matching run of `)` after it, that the formatted
  stream lacks, is skipped, and only when the remaining tokens then align one to one;
  any other divergence stays an error and the file is left unformatted. The
  tolerance is never generic. Locked by fixtures that prove the alignment before and
  after formatting, by re-running the preprocessor on the formatted output and
  comparing the marked-token positions, and by cases with many `!`, nested targets,
  redundant parentheses and comments. This also yields the one-layer canonical form for Q2;
  **there is no custom formatting pass**, `go/format` stays the authority.
- Tests: a conversion inside a nested expression, several conversions in one
  statement, multiline, comments around, redundant parentheses, idempotence
  `fmt(fmt(s)) = fmt(s)`, and no unrelated formatting change.
- `transpiler.Transpile` stays textual; the output is `(T)(x)`; no helper, import
  or new file.

### WP3. `internal/checker`
Places: `checker.gon` (variable initialiser, assignment, return, call),
`construction.gon` (field and element literals), `interface_contracts.gon`
(`isNonNilSource`, D3, `callArgParamType`), `type_coverage.gon`
(`validateNonNilKind`).
- Recognise a conversion (go/types: `Fun` is a type; the `!` offset is in
  `NonNilOffsets`); GN001 for `nil`; GN003 for a non-nilable target.
- `isRecognisedSource` (the Part A set); `isNonNilSource` gains the conversion so
  that `(!I)(rc)` satisfies D3d. No `append`, no slice expression, no inference, no
  propagation across types.
- `flowOutcome(expr, slotType)` as **one pure function** over the matrix of §2 (the
  order of checks or the location of the slot does not decide the result), called
  from every slot site; without type information it stays silent.
- `.gna` slots are part of the matrix; the implementation decision follows the
  measurement of §5.
- WP3 passes the audit only after the whole matrix of §2 is locked in fixtures.

### WP4. Protocol, GW006, fixtures, conformance
- GW006 is a warning consistently; JSON is stable and follows schema v1; the exit
  code does not change; `source`, `range` and `message` do not change without
  reason.
- Conformance fixtures test the **final output** (JSON and exit), not the presence
  of a code: warning-only (exit 0), mixed with an error, `.gna`, an interface stays
  GN001 (not GW006), a valid conversion is silent, a non-identical type does not
  inherit the contract.
- Documents: `diagnostic-protocol-v1.md` (code table), README (diagnostic table, the
  new form), CHANGELOG.
- LSP: a warning maps generically to severity 2; verification only.

### WP5. Documentation and amendments
I6 and D6b as in RFC §9, without widening Part A into Part B, and one-to-one with
the implemented rules (a static claim, no runtime check, Go decides validity, strict
identity, an interface stays GN001, `!!T` only in type positions, a conversion
triggers no binding).

### WP6. Editors and examples (after a release; not premature)
- `gon-vscode` and `gon-examples` follow syntax that has **already been released**;
  their CI is proved with the pinned released version, not a local binary. The
  TextMate grammar probably does not change (one `!` rule for types); verify with a
  fixture.
- New examples become part of the migration after a compatible pin.

### WP7. Dogfood migration of 10 sites (after a release and a pin)
Order of the `gon check ./...` gate: (1) the Part A checker is released; (2) the CI
pin uses that version; (3) the baseline of `gon check ./...` is known; (4) the
migration; (5) the gate is added to CI; (6) CI fails for a diagnostic that the
migration criterion does not allow. The gate is not installed before a compatible
pin.
Criteria: exactly the 10 Part A sites (`annotations.gon:135,172,173`,
`checker.gon:81,82`, `ecosystem.gon:346` (the conversion moves to `:317`),
`ecosystem.gon:486`, `package.gon:272,273`, `trace.gon:51`); an explicit migration;
every GW006 evaluated; no unexplained GN001 or GN003; no Part B behaviour; the
final count matches the RFC prediction or the difference is documented and approved.
This is the post-release migration gate of §4; the pre-release gate is the run of the candidate against the unmigrated sites.
**An 11th site is not migrated at once**: first classify whether it is Part A or a
side effect of Part B.
The prediction of 10 sites is kept as an audit checklist (file:line), not only a
number.

## 4. PRs and CI strategy

One new branch from the latest `main` per change; every PR audited and CI green.

| # | PR | Content |
|---|----|---------|
| 0 | conformance / tests first | harness + the fixtures of the §2 matrix + a traceability manifest; **no change of checker behaviour** |
| 1 | syntax | WP1 + WP2: `(!T)(x)` recognised and stripped, `!!T` canonical; no new diagnostic |
| 2a | conversion semantics | WP3, the conversion part: GN001/GN003, recognised sources, D3d via the conversion; no GW006 |
| 2b | GW006 | the rest of WP3 + WP4 + WP5; **only after the `.gna` measurement of §5** |
| R | release | a separate decision after 2a and 2b are integrated and the fixtures pass; not automatic |
| P | pin | the three repositories to the released version |
| M | migration | WP7 |

**Keeping `main` green (PR 0):** fixtures and the harness are added with a status per
entry in a manifest: `enabled` (already true on `main`, for example an interface
stays GN001, D3b for type parameters, Part B forms are not sources) or
`pending-PRn` (waiting for the feature). The harness runs only `enabled` entries;
the `pending` entries are listed in an allowlist that a test checks (its size must
equal the manifest, so an entry cannot disappear silently). Each implementation PR
moves entries to `enabled` together with its code; the last PR requires
`pending = 0`. The specification is therefore not weakened and `main` does not go
red. No test is switched off or has its assertions reduced.

**Compatibility of PR 0 to PR 2b with the pinned gon (v1.20.0).** The pinned gon
cannot parse `(!T)(x)`. Rule: no PR in this sequence may put the new syntax in a
`.gon` that the pinned gon reads, that is, the production packages under `cmd/`,
`internal/` and `transpiler/` that `scripts/check-gon-sources.sh` transpiles. The new
syntax may appear only in `testdata/` fixtures and in Go test string literals, which
are processed by the gon built from the branch and not by the pinned one. The
`gon-sources` job must stay green with v1.20.0 in every PR; a PR that would break it
is not mergeable.

**Q7, decided: two gates.**
1. *Pre-release gate*, on a development branch with PR 0 to 2b integrated and a
   locally built candidate (nothing of it merged to `main`): run the candidate
   against the 10 unmigrated dogfood sites and verify the GW006 result against the
   RFC prediction; audit the result and the candidate's CI.
2. *Post-release migration gate*, after R and P: migrate the 10 sites, add the
   `gon check ./...` gate to CI, and require the CI to use the compatible pin.
The migration is never required to be merged before the release, since that would be
circular (the pinned gon cannot parse the migrated sources).

## 5. Measurement of `.gna` before PR 2b

Required: `gon-examples`, the `gon-vscode` fixtures (`ecosystem*`), and `.gna`
fixtures in Gon if any; identify false positives; compare the RFC prediction with the
result. The temporary instrument must model `.gna` contracts (reading `.gna` through
the `gna` package); the result is a separate report. The result does not reopen the
semantic decision; it fixes the fixtures and the migration impact. An unexplained
difference puts the GW006 implementation on HOLD.

## 6. General acceptance criteria
`gofmt -l .` clean (apart from the existing non-Go fixture), `go vet ./...`,
`go test ./... -count=1`, `test-os` green, `scripts/check-gon-sources.sh` green with
the pinned gon, new tests fail on a meaningful mutant (the mutant must compile), and
the result reported per job as it is.

## 7. Risks and limits
A context-based preprocessor (the differential is mandatory); `CleanToOrig` and
`NonNilOffsets` after extra `!` are removed; redundant parentheses in `fmt` (WP2);
new warnings for consumers (only `diagnostics`, not the exit code); the v1.20.0
rebinding defect of `:=` is accepted as a limit of Part A; `.gna` not yet measured;
the temporary measurement instrument is not in the repository (the differential and
the dogfood check must be runnable from the repository); no time estimate.

## 8. Still out
Part B, the rebinding of `:=`, `--strict`, GW007, `&x`, inference from a body,
runtime checks, flow analysis, and the release shape without a separate decision.

## 9. Remaining items

- **Q7 and Q8 are decided** (§0, §4, WP2); this plan has no other open question.
- **A clarification of RFC §6.1 is suggested, for the owner to decide.** §6.1 requires
  the preprocessor to recognise `(` `!` TypeExpr `)` `(` as a conversion, and also to
  leave any valid Go program unchanged; for `adder(!flag)(3)` the two requirements
  conflict unless the `(` is a grouping parenthesis. WP1 adopts that precondition.
  The RFC text should say so, as a clarification and not as a change of the design.
