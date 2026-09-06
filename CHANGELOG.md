# Changelog

## [1.5.0] — 2026-09-06

### Added

- **Ecosystem Contract Expansion** — M4a + M4b + M5a
  (`docs/rfc-ecosystem-contract-expansion.md`). External `.gna` `types:`
  field contracts are now applied across the package boundary at the same
  three sites as local field contracts.
  - **M4a — non-interface application.**
    - A keyed `pkg.T{…}` literal that writes an explicit `nil` for a `!`
      field is **GN001** (previously silent; GN002 for a *missing* `!` field
      was already emitted).
    - `new(pkg.T)`, `&pkg.T{}`, and `pkg.T{}` are zero-value construction
      sites: a `!` field left at zero is **GN002**, including a `!` field
      **promoted from an embedded external type** (one hop).
    - A selector `x.F` where `x` has external type `pkg.T` and `.gna` marks
      `T.F` as `!` is a **non-nil source**: `x.F == nil` → **GW001**; `x.F`
      into a `!U` target is accepted. The non-nil-source recogniser now also
      accepts `!`-field selectors for local types (previously a gap).
    - `x.F = nil` into an external `!` field is **GN001**.
    - Unkeyed `pkg.T{a, b}` stays behind the firewall — no diagnostic in
      either direction (Gon does not reconstruct external field order).
    - External `types:` resolution is **exact** (import path + type + field);
      the local name-only field heuristic is never used across the boundary.
  - **M4b — interface-typed application.** An external contract position
    (param, result, or `types:` field) whose Go type is an interface carries
    `docs/rfc-interface-semantics.md` semantics: `!I` = interface value
    non-nil; an ordinary interface-typed value into that position is
    **GN001** (D3b); a concrete/typed-nil operand is accepted (D3a).
  - **M5a — `.gna` validation against the real package.**
    - **GN003** — the `.gna` `params`/`results` count for a resolved
      function or method must equal the Go signature's (a variadic final
      parameter counts as one). On mismatch the entry is **dropped** (its
      `!` flags no longer apply) so a stale annotation cannot shift a claim
      onto the wrong argument.
    - **GW004 (new code, warning)** — a `.gna` key under `functions:`,
      `methods:`, or `types:` that names a symbol the package does not
      provide. `methods:` validation uses the full `go/types` method set, so
      methods promoted from an embedded field are accepted.
    - Validation is best-effort (only for packages `go/types` resolved) and
      reports each condition **once per annotated symbol per run**.

### Changed

- `gon version` reports `1.5.0`.

### Compatibility

- `.gna` schema remains **1** — no file-format change.
- One new diagnostic code: **GW004** (warning; contributes exit 0).
- **Possibly breaking** in three narrow ways, each surfacing a
  previously-silent defect: a `.gna` with the wrong arity now emits GN003
  (was: silently shifted contract); a `.gna` naming a missing symbol now
  emits GW004; code that writes explicit `nil` into — or relies on flow
  reasoning about — an external `!` field now emits GN001/GW001.
- Code importing only unannotated packages, or correct `.gna` files used
  correctly, is unaffected.
- Still no type coverage, no generics, no flow-sensitive nilability, no
  dynamic-value tracking. `receivers:` remains parsed and reserved (no
  call-site effect). `--strict` is deferred to a follow-on.

## [1.4.1] — 2026-09-06

### Fixed

- **`gon fmt` mis-placed `!` on re-insertion.** The formatter stripped `!`,
  ran `go/format`, then re-inserted `!` before the *Nth textual occurrence*
  of each type name. When one type appeared as both `!T` and `T` in the same
  file (common with v1.4 interface contracts — an ordinary `I` value flowing
  into `!I`), every `!` landed on the wrong occurrence: a silent semantic
  corruption of the source.
- Re-insertion now walks the clean and formatted **token streams** in
  lockstep (comments and semicolons dropped; a `go/format` trailing-comma
  change is resynced), so the Nth marked type token maps to its exact
  counterpart regardless of textual repetition. If the streams cannot be
  aligned, `gon fmt` returns an error and leaves the file untouched instead
  of writing a corrupted result.

### Changed

- `gon version` reports `1.4.1`.

### Compatibility

- No semantic change. Diagnostic codes, severities, checker behaviour, and
  Diagnostic Protocol v1 are all unchanged from v1.4.0.
- `.gna` schema remains **1**.
- A file that `go/format` leaves byte-identical now round-trips through
  `gon fmt` unchanged.

## [1.4.0] — 2026-09-05

### Added

- **M1a — Interface non-nil contracts** (`docs/rfc-interface-semantics.md`).
  `!I` means the *interface value* is non-nil; it never constrains the
  interface's dynamic value.
  - **D3a** — a concrete/typed value assignable to `I` satisfies `!I`, even
    when it is itself typed-nil.
  - **D3b (new rejection)** — an ordinary interface-typed value cannot
    satisfy `!I` (GN001), unconditionally. An explicit `!= nil` check does
    not narrow it. Enforced at `var` init, plain assignment, `return`, and
    `!I` call arguments.
  - **D3c** — `nil` literal → `!I` is GN001 (unchanged path).
  - **D3d / D4** — an existing `!I` binding, or a result declared `!I`,
    satisfies `!I` and is a non-nil source at the call site — but only for
    the **same** interface type (`types.Identical`). `!ReadCloser` is not a
    `!Reader` source and vice versa: `!` does not propagate across an
    embedded/embedding interface (§3.7). Enforced with the resolved target
    type at all four sites: `var` init, plain assignment, `return`, and
    `!I` call arguments.
  - **D6a** — `x.(!I)` (interface assertion target) is rejected with GN001
    instead of a parse error; the preprocessor now recognises the `!` in a
    type-assertion target. `x.(!*T)` (concrete) is left for a later RFC.
  - **D6c** — an explicit conversion `I(x)` has interface static type and
    cannot satisfy `!I`; the checker does not look through the conversion to
    the concrete operand.

### Changed

- `gon version` reports `1.4.0`.

### Compatibility

- No `.gna` schema bump (schema remains **1**). `!` on interface types was
  already valid syntax; this release only defines its semantics.
- Existing GN001/GN002/GW001/GW002/GW003 codes and severities preserved.
- The only new rejection is ordinary `I` → `!I` (and `nil`/`x.(!I)` in the
  same position). Code that does not put a `!` on an interface type is
  unchanged.
- Still no type coverage, no generics, no flow-sensitive nilability, no
  dynamic-value tracking.

## [1.3.0] — 2026-08-19

### Added

- **M2a construction completeness**
  - `new(T)` is a zero-value construction site when `T` is a local struct (GN002).
  - Unkeyed composite literals use declaration order from the *local* struct AST only.
  - External (`SelectorExpr`) types remain keyed-only + `.gna` (M4 firewall).
  - Unified construction entry points in `internal/checker/construction.go`.
- **ContractTrace** — first-class diagnostic data on GN001/GN002 (wire `String()` unchanged).
- **GW003** — redundant type-assertion warning on declared `!T` identifiers only.
- **`gon fmt`** — format Gon source in place.
- **`gon lsp`** — minimal stdio LSP.

### Changed

- `gon check` remains canonical; `vet` stays a compatibility alias.
- `gon version` reports `1.3.0`.

### Compatibility

- No `.gna` schema bump (schema remains **1**).
- Existing GN001/GN002/GW001 codes and severities preserved.
- Scope firewall: no interface `!I`, no type coverage, no generics, no flow-sensitive nilability.

## [1.2.1] — 2026-08-18

### Fixed

- **Preprocessor: `!` in multi-result function signatures.** Local forms such
  as `func Open() (!*string, error)`, `func Open() (*string, !error)`, and
  `func F(a !*T) (!*U, error)` are recognized as type modifiers. Parameter
  and parenthesized result lists (including interface methods) are tracked so
  unary `!` in expressions (`f(a, !b)`, `(!flag)`) is not stripped.

### Compatibility

- No checker semantics change relative to v1.2.0.
- Field contracts and return-value contracts unchanged.
- `.gna` schema remains **1**.

> Gon v1.2.1 is a preprocessor bugfix so valid local multi-return `!`
> syntax works as intended since v1.1.

### Changed

- `gon version` reports `1.2.1`.

## [1.2.0] — 2026-08-17

### Added

- **Field contracts.** A `!T` field is an invariant of its declared storage
  location. Enforced at three local sites only:
  - **Construction (GN002):** zero-value containment walk over struct,
    embedded, fixed array, and anonymous types; stops at every indirection
    boundary (`*T`, `[]T`, `map`, `chan`, `interface`, `func`).
  - **Mutation (GN001):** explicit `nil` assigned into a `!T` field
    (`s.Client = nil`). Ordinary values remain accepted (conservative).
  - **Selector (non-nil source):** selecting a `!T` field yields a non-nil
    source for immediate use (GW001 on nil comparison).
- **`.gna` `types:` section.** External packages may declare field contracts:
  ```yaml
  types:
    Config:
      fields:
        Client: "!*http.Client"
  ```
- Spec: [docs/rfc-field-contracts.md](docs/rfc-field-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change; `types:` was already
  syntactically open under schema 1).
- Programs accepted by v1.1 remain accepted unless they construct or mutate
  a value that violates a newly annotated `!` field.
- No flow analysis, no path sensitivity, no reconstruction of object state.
- Ordinary value into `!T` field remains accepted.

> Gon v1.2 makes `!T` fields storage invariants checked at construction,
> mutation, and selector use — still local and non-flow-sensitive.

### Changed

- `gon version` reports `1.2.0`.
- Scope document updated for v1.2 semantics.

## [1.1.0] — 2026-08-17

### Added

- **Return-value contracts.** An explicitly annotated `!T` result position
  (in `.gna` or a local Gon signature) becomes a non-nil source at its
  immediate use site (`:=` or `var` without an explicit type). Observable via
  GW001 on nil comparison.
- Spec: [docs/rfc-return-value-contracts.md](docs/rfc-return-value-contracts.md).

### Compatibility

- `.gna` schema remains **1** (no format change).
- Ordinary (unannotated) values assigned to `!T` remain accepted.
- No flow analysis, no conditional contracts, no propagation through
  conversion / type assertion / assignment from names.
- Explicit type on a binding always wins over a return contract.

> Gon v1.1 adds explicit return-value contracts without introducing flow
> analysis. Annotated `!T` return positions become non-nil sources at their
> immediate use sites; existing ordinary-value acceptance remains unchanged.

### Changed

- `gon version` reports `1.1.0`.
- Scope document updated for v1.1 semantics.

## [1.0.0] — 2026-08-16

Initial public release.

- Local non-flow-sensitive nilability checks (`!T`).
- External packages via `.gna` + module-aware `go/packages`.
- Conservative reference annotations for `io` and `os`.
