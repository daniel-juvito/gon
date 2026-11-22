# Gon Roadmap: v1.3 and Beyond

**Status:** Provisional working release plan. Release boundaries beyond v1.4
may shift based on RFC outcomes, but the sequencing principles (R1–R4) and
dependency graph are locked.

## 1. Semantic Invariants (I1–I7)

These are the normative gate for every feature on this roadmap. A feature
that violates an invariant is REJECTED or must propose an invariant change
via its own RFC.

- **I1 — No flow-sensitive nilability.** Gon does not change nilability
  state based on control-flow path. Scope: nilability-state only; generic
  reachability/dead-code analysis is out of scope for this invariant.
- **I2 — Explicit contracts are authoritative.** Non-nil guarantees come
  from explicit commitment, never body inference.
- **I3 — Go semantics remain authoritative.** `go/types` is the authority
  for type identity, assignability, conversions, method sets, and ordinary
  Go semantics.
- **I4 — `.gna` is authority only for external nilability contracts.**
  Identity and ordinary type semantics remain under I3, including for
  packages whose contracts are described by `.gna`. *(Amended in v1.14 / M10,
  owner 2026-11-17: an imported module-local package's contracts may come from
  its `.gon` sources, which take precedence over a `.gna` for the same
  package; limited to the main module. Wording:
  `docs/rfc-source-contracts.md` §10.)*
- **I5 — `!T` is a compile-time guarantee**, not a new runtime
  representation or runtime nil-check.
- **I6 — No implicit contract propagation** through conversion, assertion,
  or ordinary assignment. `!T → T` may lose the guarantee; `T → !T`
  requires a source that satisfies the contract at that point. A `T → !T` flow
  of a non-interface nilable kind is reported as a warning (GW006) in two cases:
  when the source is not a recognised non-nil source, and when the source is
  recognised but its static type is not identical to the type of the `!U` slot even
  though Go accepts the assignment. The explicit non-nil conversion `(!U)(x)` is a
  recognised non-nil source of type `U`; it is a claim by the author and is not
  checked. *(Amendment of `rfc-explicit-nonnil-conversion.md` §9, Part A; unreleased.)*
- **I7 — Schema versioning is syntax-driven.** Semantic/feature changes do
  not by themselves bump the `.gna` schema version. `Requires .gna change
  = Yes` and `Schema bump = No` is a valid combination.

## 2. Release Sequencing Principles (R1–R4)

- **R1 — Semantic Isolation.** A potentially breaking semantic RFC MUST be
  released independently from unrelated semantic RFCs so its compatibility
  impact can be observed in isolation.
- **R2 — Non-breaking Bundling.** Non-breaking extensions MAY be bundled
  when they form a coherent maintenance/tooling/construction story.
- **R3 — Dependency Preservation.** Release slicing MUST NOT weaken or
  bypass dependencies established by the locked dependency graph.
- **R4 — No Artificial Versioning.** Independent non-breaking work does
  not require a dedicated minor release merely because it constitutes a
  separate milestone.

A feature's `Breaking = Possibly Yes` label does not automatically mean a
solo release — that determination is made by the RFC's own compatibility
analysis (see v1.6, §4).

## 3. Milestone Map

| ID | Scope | Dependency / Gate |
|---|---|---|
| M1a | Interface semantics + method-set contracts | Core — **shipped v1.4** |
| M1b | Type Coverage semantics | Core — **shipped v1.6** |
| M2a | Struct-only construction + diagnostics + warnings | Core |
| M2b | Element-contract construction subcases | M1b — **shipped v1.7** |
| M3 | Generic `!T` + instantiated generic contracts | M1a (full) + M1b named-type nilability semantics — **shipped v1.8** |
| M4a | Cross-package / External resolution + non-interface application | Core + basic `.gna` model — **shipped v1.5** |
| M4b | Interface-typed contract application | M1a + M4a application model — **shipped v1.5** |
| M5a | `.gna` validation + external contract boundary | M4a — **shipped v1.5** |
| M5b | Generic `.gna` + generic contract application | M3 + M5a infrastructure — **shipped v1.9** |
| M6a | `gon fmt` + `gon check` | Core |
| M7 | Call-site contracts for source-declared methods | M3 + M5b + M8 — **shipped v1.11** |
| M8 | Package-aware checking | M5b — **shipped v1.10** |
| M9 | Directory form for `transpile` / `build` | M8 — **shipped v1.12** |
| M10 | Cross-package contracts from module-local `.gon` sources | M8 — **shipped v1.14** |
| M6b | LSP / editor support | M2a — contract tracing |

Full feature inventory and dependency graph rationale are maintained
separately in the project's design-session history and are not duplicated
here.

## 4. Release Plan

### v1.3 — Foundation & Tooling
**Scope:** M2a + M6a + M6b — construction completeness, diagnostics,
warnings, CLI (`gon fmt`, `gon check`), and LSP/editor support.
**Dependency:** Core only.
**Breaking risk:** None.
**Rationale (R2/R4):** All non-breaking. No reason to withhold or split
into separate releases — this is a coherent "strengthen the existing
checker/toolchain" story.

### v1.4 — Interface Semantic Gate — SHIPPED (2026-09-05)
**Scope:** M1a — Interface RFC (`!I` semantics, typed-nil behavior,
method-set contracts). Implemented per `docs/rfc-interface-semantics.md`
(Status: Accepted). Observed compatibility impact: one new rejection —
ordinary `I` → `!I` (with `nil` and `x.(!I)` in the same position). No
`.gna` schema bump. No change to code that does not annotate an interface
type with `!`.
**Dependency:** Core.
**Breaking risk:** Possibly Yes — highest fan-out of any pending RFC.
**Rationale (R1):** Solo release, mandatory. This decision gates M3, M4b,
and downstream ecosystem work; its compatibility impact must be observable
in isolation before anything is built on top of it.

### v1.5 — Ecosystem Contract Expansion — SHIPPED (2026-09-06)
**Scope:** M4a + M4b + M5a — cross-package and external `.gna`
resolution/application (non-interface, then interface-typed), plus `.gna`
validation against `go/types` (arity → GN003 + contract drop; unknown
symbol → new **GW004**) and the external contract boundary.
**RFC:** `docs/rfc-ecosystem-contract-expansion.md` (Status: Accepted —
E1–E14 + §10 locked 2026-09-06). One-hop external structural traversal;
`--strict` deferred to a follow-on; `receivers:` still reserved.
**Dependency:** M4a: Core + basic `.gna` model. M4b: M1a (v1.4, shipped). M5a: M4a.
**Breaking risk:** M4a/M4b: Possibly Yes (ecosystem compatibility
exposure). M5a: non-breaking.
**Rationale (R1, applied correctly):** M4b is *application* of a semantic
decision already locked in v1.4, not a new semantic RFC — it does not
independently trigger R1 isolation. Bundling M4a+M4b+M5a as one coherent
"contracts cross package boundaries" story is intentional sequencing, not
readiness-driven bundling. M4a was deliberately not pulled into v1.3
despite being dependency-eligible from Core alone, to preserve each
release's single clear character.

### v1.6 — Type Coverage — SHIPPED (2026-09-06)
**Scope:** M1b — Type Coverage RFC (`!func`, `!map[K]V`, `![]T`,
`!chan T`, named nilable types, aliases).
**RFC:** `docs/rfc-type-coverage.md` (Status: Accepted — decision matrix
C1–C14 + §10 O1–O7 locked 2026-09-06, owner confirmed all recommendations).
**Dependency:** Core.
**Breaking risk:** One materially breaking sub-feature (C5 option P: bare
`var x !S` with no initializer → GN002, uniform across nilable kinds,
closes the pre-existing silent `!*T` gap). Every other sub-feature (C3,
C6, C7, C9, C11) is non-breaking in practice.
**Rationale:** Defaults to solo per R1, and the RFC's §13 recommendation is
to keep it solo: C5(P) is the isolation-worthy change; the non-breaking
rows ride along as the coherent "`!` means the same thing on every nilable
type" story (R2). If the owner takes C5 option Q (status quo) the release
becomes fully non-breaking and folding M1b into v1.7 with M2b is on the
table (RFC O1/O7).

### v1.7 — Element-Contract Construction — SHIPPED (2026-09-07)
**Scope:** M2b — element-contract construction subcases.
**RFC:** `docs/rfc-element-contract-construction.md` (Status: Accepted —
E1–E12 locked, O1/O4/O5 deferred, 2026-09-07).
**Dependency:** M1b element-level semantics (v1.6).
**Breaking risk:** Non-breaking — every new diagnostic (GN001 nil element,
GN002 fixed-array element zero-fill, GN003 malformed element `!`) fires
only on syntax v1.6 rejected. `.gna` schema stays 1.
**Rationale (R2):** Follow-on extension; semantic risk was already
absorbed in v1.6, so no isolation needed here.

### v1.8 — Generic Semantics — SHIPPED (2026-11-14)
**Scope:** M3 — Generic RFC (`!T` type parameters, instantiated generic
contracts).
**RFC:** `docs/rfc-generic-semantics.md` (owner decisions O1–O7 accepted
2026-11-13; implemented 2026-11-14 — two corrections in RFC §13.1 confirmed
by the owner 2026-11-14).
**Dependency:** M1a full (v1.4) + M1b named-type nilability semantics (v1.6).
**Breaking risk:** Possibly Yes.
**Rationale (R1):** Solo release. Independent compatibility exposure even
though it depends on prior RFCs.

### v1.9 — Generic `.gna` Ecosystem — SHIPPED (2026-11-14)
**Scope:** M5b — generic `.gna` declarations + generic contract
application.
**RFC:** `docs/rfc-generic-gna.md` (Status: Implemented in Gon v1.9,
2026-11-14; owner decisions Q1–Q7 recorded, Q2 / Q3 after an exercise).
Measured baseline: most generic `.gna` application already worked by name +
position; the release closes the explicit-instantiation inconsistency and
validates `.gna` `!` positions against `go/types` (type parameters via the
M3 nilable rule; concrete kinds per `gna-spec-v1.md`).
**Dependency:** M3 (v1.8) + M5a infrastructure (v1.5).
**Breaking risk:** Possibly Yes (inherited from M3).
**Rationale (R1):** Follows the isolation of M3 as its direct consumer.

### v1.10 — Package-Aware Checking — SHIPPED
**Scope:** M8 — `gon check ./dir` and `./...` type-check a package's `.gon`
files together in their real directory, share contracts across them, and
report each diagnostic against its own file. File arguments are unchanged.
**RFC:** `docs/rfc-package-check.md` (Status: Implemented — owner decisions
Q1–Q7 recorded 2026-11-15).
**Dependency:** M5b (v1.9).
**Breaking risk:** None (additive: a directory argument was an error before).
**Rationale (R4):** a distinct, isolated feature with its own regression suite;
ships solo.

### v1.11 — Source Method Contracts — SHIPPED
**Scope:** M7 — a `!` on a method's parameter or result, written in the checked
package, is enforced at its call sites, matched by `go/types` object identity.
**RFC:** `docs/rfc-method-contracts.md` (Status: Implemented — owner decisions
Q1–Q6 accepted as proposed, 2026-11-16). Sequenced after M8 on purpose: its
single-file scope limit (S9) widens to "the package" with M8's package mode.
**Dependency:** M3 (v1.8) + M5b (v1.9) + M8 (v1.10).
**Breaking risk:** Possibly Yes.
**Rationale (R1):** solo release.

### v1.12 — Directory Form for `transpile` / `build` — SHIPPED
**Scope:** M9 — `gon transpile <dir>` / `dir/...` and `gon build <dir>` check the
directory as one package (the M8 machinery) and write `.go` only if it passes;
in-process `go/format` replaces the `gofmt` on `PATH`. File arguments unchanged.
**RFC:** `docs/rfc-transpile-dir.md` (Status: Draft until audit — owner
decisions Q1–Q6 accepted as proposed, 2026-11-16).
**Dependency:** M8 (v1.10).
**Breaking risk:** None (additive: a directory argument was an error before).
**Rationale (R4):** the item M8 left out; needed before a multi-file codebase,
this repository included, can be written in Gon. Ships solo.

### v1.13 — Return-Position Contracts — SHIPPED
**Scope:** a coverage fix, not a new feature — the operands of a `return` are
checked like every other expression (calls, composite literals, `new(T)`,
comparisons, type assertions) and a function literal is checked as a function
(its own scope, parameters and result contract). No RFC: it closes undocumented
gaps; the closure part was found in the audit. Found while measuring the dogfood conversion (M9).
**Dependency:** none.
**Breaking risk:** Possibly Yes (R1): code that violates a `!` contract inside a
`return` operand is reported now. A differential over the three repositories'
`.gon` files and a 526-file standard-library corpus found no new diagnostic.
**Rationale (R1):** solo release.

### v1.14 — Cross-Package Contracts from `.gon` Sources — SHIPPED (2026-11-17)
**Scope:** M10 — an importer sees the `!` contracts of an imported main-module
package that has `.gon` sources, derived from those sources and applied through
the existing `.gna` machinery. Amends invariant I4. New warning GW005.
**RFC:** `docs/rfc-source-contracts.md` (owner decisions Q1–Q6 and the I4
amendment, 2026-11-17). Implemented and audited in #38.
**Dependency:** M8 (v1.10); motivated by the dogfood conversion.
**Breaking risk:** Possibly Yes (R1).
**Rationale (R1):** solo release.

### v1.15–v1.20 — Exit Codes and Package Discovery — SHIPPED
No new milestone: an audit of `gon check --json`'s exit codes (B) and then of
package discovery, each change measured against minimal reproductions,
proposed, audited, and released solo because each one changes exit codes a
consumer can see (R1). Diagnostic Protocol v1 is unchanged throughout (same
`schemaVersion`, envelope and codes); §7 of `docs/diagnostic-protocol-v1.md`
records every rule.

- **v1.15 (#42, 2026-11-17)** — a package that cannot be loaded (no `go`
  command, invalid `go.mod`, a file excluded by build constraints) is exit 2,
  not 1; an invalid `package` clause in package mode is a parse error with an
  envelope. §7 separates the file-mode pre-check parse failure, package-mode
  loader errors, and package-loading failures, and states all-or-nothing.
- **v1.16 (#46, 2026-11-18)** — the go command decides which `.gon` files
  belong to a package inside a module (an excluded `.gon` is not checked; a
  package with no source in the build is exit 2 when named, skipped under
  `...`); `...` stops at a nested module; `transpile` / `build` write only the
  files that were checked. Corrects two claims of 1.15.0.
- **v1.17 (#48, 2026-11-18)** — file mode no longer falls back silently when a
  `go.mod` is found but its module cannot be loaded: exit 2, "invalid go.mod"
  when golang.org/x/mod proves it, the go command's error otherwise. The
  importer fallback remains only when no `go.mod` is found.
- **v1.18 (#49, 2026-11-19)** —
  package mode outside a module loads every file of the package (membership
  by go/build; `GOFLAGS` tags are not applied there); build constraints apply as inside a module; a malformed
  `//go:build` line is a Go error of the package; import resolution outside a
  module is unchanged.
- **v1.19 (#52, 2026-11-19)** — inside a module, a `.gon` with a malformed
  `//go:build` line is a source error of its package, not a load failure: the
  file is not checked, the package's other files are, the run is exit 1 and
  stderr names the file and go/build's reason. It is not a Diagnostic (no
  `code`, no `range`). A name the go tool ignores (`_x.gon`, `.x.gon`) is not
  part of the package and is no error. Outside a module the rule of v1.18
  stands, with the same message.
- **v1.20 (#54 and #56, released in #60, 2026-11-21)** — two changes in one
  release. Each was merged on its own, and the second landed on the first, so
  they ship together rather than as two solo releases (a conscious departure
  from the R1 precedent of this series).
  - **File mode (#54).** A `.gon` **file operand** with a malformed
    `//go:build` line is the same source error as in v1.19: not checked, the
    other operands are, one envelope, exit 1, nothing written by `transpile` /
    `build`, and `fmt` is unchanged. go/build is asked about the contents under
    a neutral file name, so the operand's own name does not decide.
  - **The module context of `gon build <dir>` (#56; amends RFC M9 T4).**
    `build` transpiled every package and then ran `go build`, which failed for
    want of a module with the `.go` files already written. Now, when it can
    prove that no module is in force where `go build` runs, or that the operand
    is outside the main module it uses, it refuses before anything is checked
    or written: exit 2, nothing on stdout, one of two messages. Whatever it
    cannot prove is left to `go build`, as before; an active workspace is not
    predicted (`go.work` is not read). Only the directory form of `build` is
    affected.

**Breaking risk:** Yes for exit-code consumers (each release lists the
changed outcomes in `CHANGELOG.md`). **Rationale (R1):** one solo release per
change, except v1.20, which carries two.

## 5. Open Sequencing Notes

- **Open work after v1.20 (none scheduled, none audited as done).**
  - **C — no RFC yet.** Cross-package contracts on a defined type whose base
    comes from another package, and checking a module-local dependency that
    has only `.gon` sources (no `.go`) before it is transpiled. Deferred by
    the owner; M10 (v1.14) covers neither.
  - **D — receiver contracts, no RFC yet.** `receivers:` in `.gna` is parsed,
    validated and still reserved (no call-site effect since v1.5); a `!` on a
    source receiver is not part of M7 (`rfc-method-contracts.md`).
  - **`--strict`** — deferred since v1.5 (`rfc-ecosystem-contract-expansion.md`);
    unscheduled.
  - **A "nilable" constraint** — open, unscheduled (see the note below).
  - **Explicit non-nil conversion — Part A accepted and implemented on `main`, not
    released; Part B open** (`rfc-explicit-nonnil-conversion.md`). **Part A is
    accepted as a design decision (2026-11-21) and implemented, unreleased (syntax,
    checks of a conversion, GW006, see the CHANGELOG `[Unreleased]`):** the explicit conversion `(!T)(x)` as an
    author's static claim with no runtime check (Go decides its validity), GW006 as
    a warning for an ordinary `T → !T` flow of a non-interface nilable kind
    (pointer, slice, map, channel, function; an ordinary interface value into `!I`
    stays GN001 and D3b is unchanged for type parameters), the identity rule of the
    contract, and idempotence of `!` in type positions. **Part B is open and not
    accepted:** binding of locals by inference from a structural initialiser, with
    the `append` and slice-expression sources, which is potentially breaking. The
    acceptance of Part A is not approval to implement it and sets no release
    number, date or schedule; the release shape and the migration of Gon's own
    sources (ten `(!T)(x)` sites) are still decisions of the owner.
- **Dogfood (not a release track).** Gon's production packages are written in
  Gon (`docs/dogfood.md`); `!` contracts are added package by package after
  measurement and audit (all of `internal/checker`'s approved groups are in).
  A release is needed only when the dogfood gate needs a checker it does not
  pin yet. The gate and the two companion repositories (`gon-examples`,
  `gon-vscode`) are pinned to v1.20.0.
- **Cross-OS tests (CI, not a release track).** The `test-os` job of `ci.yml`
  (#58) runs `go vet` and `go test` on Windows and macOS besides the Linux gate.
  It is informational and not a required check. Its first run on Windows failed
  in 16 tests, all assumptions of the test suite about the platform, fixed in
  tests only (#58). Its first run on #56 failed in four of that PR's own tests:
  two expected a slash in a printed path, and two showed that ENOTDIR also
  satisfies "does not exist" on Windows, which was fixed in the code; the final
  run of #56 passed all 11 jobs on Linux, macOS and Windows.

- M4a is dependency-eligible as early as Core, independent of M1a. It is
  placed in v1.5 for release coherence (R2/R4), not because the graph
  requires it. This could be revisited if ecosystem demand justifies an
  earlier standalone release.
- v1.6 onward are planned, not committed. RFC outcomes, scope changes, or
  implementation reality may shift these boundaries without violating R1–R4
  or the underlying dependency graph.
- **Go 1.27 generic methods (toolchain resolved in v1.8.1; contracts open).**
  Go 1.27 allows methods with their own type parameters
  (`func (b Box[X]) Map[Y any](…)`). Gon parses with the `go/parser` of the
  toolchain that builds it, so a Gon built with Go 1.25 rejected such source
  at parse time while one built with 1.27 accepted it — the same Gon source
  behaved differently depending on the builder's Go version (verified
  2026-11-14 against v1.8.0). **v1.8.1 raises the build minimum to Go 1.27.0**
  (`go.mod`, CI, release), so every Gon build parses the syntax. This is a
  build-level change, not a language-compatibility one: accepting the syntax
  is not breaking (it was a parse error before), and a user's *own* Go
  version does not gate it — checking was observed to produce identical
  diagnostics with the user's `go` at 1.21.13 through 1.27.0.
  - **What already works** with a 1.27 build (checked 2026-11-14): body-level
    checks on a generic method — `!Y` on a method type parameter follows the
    G2 nilable rule (`Y any` → GN003) and `return nil` into a `!*Y` result is
    GN001.
  - **What remains open**: call-site contracts for methods (parameters and
    results) are untracked for *every* receiver, generic or not
    (`rfc-generic-semantics.md` §13.1), so a generic method's contracts are
    not enforced at its callers. Defining that needs its own RFC; candidate
    slot: with or after M5b (v1.9). Once contracts start being checked on code
    that was previously unchecked, R1/G11-style compatibility analysis
    applies.
- **A "nilable" constraint (open, unscheduled — noted 2026-11-14).** Go has no
  named constraint or supertype for "a type whose zero value is `nil`":
  `nil` is typed per kind (pointer, slice, map, chan, func, interface), and
  the only way to say it is to enumerate kinds in a type set. Verified
  against Go 1.27.0: `x == nil` compiles for a type parameter whose type set
  holds only nilable kinds (`*A | []int | map[string]int`, `~*A`) and is
  rejected for `any` and `comparable`. Gon's nilable-type-parameter rule
  (`rfc-generic-semantics.md` §3) is the same boundary, so `!X` on `X any`
  being GN003 is not an arbitrary Gon rule.
  - **Consequence.** A generic `!` over a pointer / slice / map value is
    written `[P ~*E, E any](p !P)`, `[S ~[]E, E any](s !S)`,
    `[M ~map[K]V, K comparable, V any](m !M)` — the idiom the standard
    library uses (in `slices` / `maps`, 64 of the 79 bare type-parameter
    positions satisfy the rule). When the caller's named type need not be
    preserved, the plain forms `!*X`, `![]E` need no type parameter at all.
  - **A library can name the common shapes — no Gon change needed.** Verified
    2026-11-14 against Gon v1.8.1: named constraints such as
    `type Ptr[E any] interface{ ~*E }`, `Slice[E]`, `Map[K, V]` and `Chan[E]`
    are recognised by the nilable-type-parameter rule (no GN003 on
    `func F[P Ptr[E], E any](p !P)`), `E` is still inferred, and a named slice
    type is preserved. It shortens `P ~*E` to `P Ptr[E]` and gives the intent
    a name; it does **not** remove the second type parameter.
  - **What a library cannot do.** One `Nilable` constraint spanning every kind
    and element type: element types must be written out, so a union such as
    `~*int | ~[]int | …` only admits those exact elements (Go rejects
    `[]string` as `does not satisfy Nilable`). Covering the general case would
    need a language-level constraint (not Gon's to add) or new Gon syntax in a
    constraint position, which is reserved as GN003 (G9 / O5 of
    `rfc-generic-semantics.md`) and needs its own RFC; it also interacts with
    I3 (Go owns type identity and constraints) and I7 (schema bumps are
    syntax-driven).
  - **Open design questions for the library route.** Where it lives (a tiny
    separate module vs a package in the Gon module, which would pull Gon's
    dependencies into users' `go.mod`); it carries no runtime code, so I5
    (`!T` is compile-time only) holds. No candidate release; revisit if the
    `[P ~*E, E any]` form proves a real adoption cost in practice.
