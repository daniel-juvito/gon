# RFC: Return-Value Contracts (Gon v1.1)

**Status:** Implemented in v1.1.0  
**Target:** Gon v1.1  
**Related:** `docs/gna-spec-v1.md`, `docs/v1-scope.md`  
**Date:** 2026-08-17

## 1. Summary

v1.0 already permits `results:` declarations in `.gna` files and in Gon
function signatures, but the checker does **not** treat those results as
non-nil sources at call sites. Consequently an annotated return is currently
a dead contract:

```go
// annotations/example.gna
functions:
  MustGet:
    results:
      - "!*Config"

// user code
cfg := example.MustGet()   // v1.0: ordinary *Config
var x !*Config = cfg       // accepted — no diagnostic
```

This RFC promotes **explicitly annotated return positions** to non-nil
sources. It introduces no new inference, no flow analysis, and no
conditional contracts.

**Normative principle:**

> v1.1 introduces no new inference. It only promotes explicitly annotated
> return positions to non-nil sources at their immediate use sites.

## 2. Motivation

- Return values are the most common place where an API can give a genuine
  non-nil guarantee (`Must*`, constructors that panic on failure, etc.).
- Authors can already write the contract; the checker simply ignores it on
  the call site.
- The change is small, explicit, and monotonic with respect to acceptance
  behaviour.

## 3. Goals

1. A result position annotated `"!T"` (in `.gna` or in a local Gon signature)
   becomes a **non-nil source**.
2. A non-nil source may be assigned to a `!U` variable / parameter / field
   (subject to ordinary Go assignability).
3. Literal `nil` returned from a function whose result is `!T` remains an
   error (already present in v1.0).
4. The checker stays non-flow-sensitive and non-path-sensitive.
5. Parameter and receiver contracts are unchanged.

## 4. Non-Goals

- Conditional contracts (`non-nil when err == nil`)
- Flow-sensitive propagation after assignment
- Automatic inference from function bodies
- Generic / type-parameter contracts
- Field annotations
- Remote annotation registries
- Any diagnostic that turns a previously accepted ordinary value into an
  error when assigned to `!T`

## 5. Proposed Semantics

### 5.1 Non-nil sources

An expression is a **non-nil source** if and only if one of the following
holds:

| Case | Non-nil source? |
|------|-----------------|
| Expression that intrinsically produces a non-nil value (`&x`, `make`, `new`, composite literal, string/number/bool literal, etc.) | Yes (existing) |
| Named variable, parameter, or result declared `!T` | Yes (existing) |
| Result position *i* of a call whose corresponding annotation is `"!T"` | **Yes (new)** |
| Selector, index, type assertion, or conversion | No |
| Result position annotated `"T"` or left unannotated | No |

### 5.2 Immediate use only

A non-nil source may be consumed in an **immediate** context that requires
non-nil:

- assignment to a `!T` variable / parameter / field
- passing as an argument to a `!T` parameter
- use as the receiver of a method call

No further propagation is performed. In particular:

```go
x := config.MustConfig()          // x is a non-nil source
y := x                            // y is ordinary (no flow)
z := (*SpecialConfig)(x)          // z is ordinary
```

### 5.3 Multi-value returns (normative)

Contracts are attached to **result positions**, never to the call expression
as a whole.

```yaml
functions:
  Open:
    results:
      - "!*File"     # position 0
      - "error"      # position 1
```

Conceptually the checker materialises:

```
result[0] → non-nil source
result[1] → ordinary
```

This holds for every form of multi-value assignment:

```go
f, err := config.Open(...)
f, _   = config.Open(...)
var f !*File
var err error
f, err = config.Open(...)
```

### 5.4 Named results

Mapping remains strictly positional. Names are documentation only.

### 5.5 Method values and method expressions

Resolution continues to use `go/types` + annotation lookup. The contract of
the resolved method is applied to the corresponding result positions.

### 5.6 Local Gon functions

A function defined in a `.gon` file with a `!T` result is a non-nil source
at call sites inside the same package. Cross-package use of such functions
requires an explicit `.gna` entry written by the author (no automatic
generation is implied by this RFC).

### 5.7 Direct receiver use

```go
config.MustConfig().Reload()
```

is accepted when `MustConfig`'s result is annotated `!*Config` (or the
receiver of `Reload` is declared non-nil). This is simply an immediate use
of a non-nil source; it does not open the door to general data-flow tracking.

## 6. Changes to the `.gna` Specification

None. The grammar and schema version stay at **1**.

`results:` entries that were already legal under schema 1 become
semantically active on call sites. Schema version numbers continue to
reflect format/grammar compatibility, not every checker semantics change.

## 7. Compatibility

- Every program accepted by v1.0 remains accepted by v1.1.
- Existing `.gna` files remain valid; authors who want the new behaviour
  simply ensure the relevant result positions carry `"!"`.
- No new diagnostic codes are required for the core feature.
- Annotating a result as `"!int"` (or any other non-nillable type) does not
  produce a new warning; existing rules about the legality of `!T` are left
  unchanged.

### Migration picture

```
v1.0
  ordinary value ───────────────→ !T   accepted

v1.1
  ordinary value ───────────────→ !T   accepted
  annotated !T return ──────────→ !T   additionally recognised as guaranteed
```

## 8. Examples

### Positive

```yaml
# annotations/github.com/acme/config.gna
functions:
  MustLoad:
    results:
      - "!*Config"
```

```go
cfg := config.MustLoad("app.yaml")
var c !*Config = cfg               // OK
process(cfg)                       // OK when process takes !*Config
config.MustLoad("app.yaml").Save() // OK (direct receiver)
```

### Still accepted (no new diagnostic)

```go
func get() *int { return nil }
var x !*int = get()                // still OK — get() is not annotated !T
```

### Still an error (v1.0 behaviour)

```go
func must() !*int { return nil }   // GN001
```

### Conservative annotations remain conservative

`os.Open` continues to leave `*File` ordinary because the real contract is
conditional. Only APIs that truly guarantee non-nil (e.g. a `MustOpen` that
panics) should be annotated `"!*File"`.

## 9. Alternatives Considered

| Alternative | Reason rejected |
|-------------|-----------------|
| Conditional contracts | Requires path sensitivity |
| Body inference | Violates "explicit contract only" |
| Making ordinary → `!T` an error | Breaking; changes the character of v1 |
| New syntax for results | Unnecessary; `"!T"` is already sufficient |
| Raising schema to 1.1 | Syntax did not change |

## 10. Open Questions (resolved for v1.1)

| # | Question | Decision |
|---|----------|----------|
| 1 | Schema version | Remain `1` |
| 2 | Direct receiver from `!T` result | Yes |
| 3 | Propagation through conversion / assertion | No |
| 4 | Warning on `!` applied to non-nillable types | No new diagnostic |
| 5 | Automatic `.gna` generation from exported `.gon` functions | Out of scope; author must write the `.gna` entry |

## 11. Implementation Notes

- When lowering a `CallExpr`, resolve the signature and, for each result
  position annotated `!`, mark the corresponding result temporary as a
  non-nil source.
- Consume that mark only in immediate contexts (assignment, argument,
  receiver).
- No data-flow graph, no path sensitivity, no inter-statement tracking.
- Test matrix must include:
  - single- and multi-value returns
  - blank identifiers
  - direct method calls on the result
  - cross-package calls via `.gna`
  - preservation of all v1.0 acceptance cases

## 12. Future Work (not part of this RFC)

- Field annotations (`types:` section)
- Generic contracts
- Optional strict mode (ordinary value into `!T` becomes an error)
- Community / remote annotation registry

---

**Locked decisions**

| Decision | Choice |
|----------|--------|
| Annotated return → non-nil source | Yes |
| Ordinary value → `!T` remains accepted | Yes |
| Flow analysis | No |
| Conditional contracts | No |
| Schema | Stay at 1 |
| Multi-return | Positional |
| Named results | Names irrelevant |
| Direct receiver from `!T` result | Yes |
| Propagation via conversion/assertion | No |
| Body inference | No |
| New diagnostic for non-nillable `!T` | No |
