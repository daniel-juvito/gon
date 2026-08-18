# Changelog

## [1.3.0-dev] — 2026-08-18

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
- `gon version` reports `1.3.0-dev` until the release tag.

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
- **`.gna` `types:` section.** External packages may declare field contracts.
