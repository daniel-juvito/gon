# .gna Specification v1

`.gna` files supply **nilability metadata** to the Gon checker.
They are not a second type system. `go/types` remains the sole authority
for Go semantics (type identity, assignability, method sets, etc.).

## Location and naming

```
<module-root>/annotations/<import-path>.gna
```

Examples:

```
annotations/io.gna
annotations/os.gna
annotations/net/http.gna
```

The import path recorded inside the file must match the path after
`annotations/`.

## Format

YAML. Every type annotation is a quoted string so that parsers treat
`!` as ordinary data, not a YAML tag indicator.

## Schema

```yaml
schema: 1                         # required; format version
package: io                       # required; import path being annotated
description: "optional prose"     # optional

functions:
  FunctionName:
    params:
      - "T"
      - "!T"
    results:
      - "T"
      - "!T"

methods:
  TypeName.MethodName:
    params:
      - "T"
      - "!T"
    results:
      - "T"
      - "!T"

receivers:
  TypeName: "!"                   # optional; marks the receiver non-nil

types:                            # optional; field contracts (v1.2)
  TypeName:
    fields:
      FieldName: "!T"
      Other:     "T"
```

### Nilability notation

| Notation in `.gna` | Meaning for the Gon checker              |
|--------------------|------------------------------------------|
| `"!T"`             | explicit non-nil claim                   |
| `"T"`              | explicit ordinary / no non-nil claim     |
| omitted            | ordinary (same as `"T"`)                 |

- The type string after `!` (or the whole string when unadorned) is
  documentation and positional matching only. The checker does **not**
  re-type-check it.
- Parameter and result matching is strictly by position.
- Method identity is `TypeName.MethodName`.
- Receiver annotations live under `receivers:` and are independent of
  the method parameter list.
- Field contracts under `types:` (v1.2) make `!T` fields storage invariants
  for the named type; see [docs/rfc-field-contracts.md](rfc-field-contracts.md).
- `"!I"` where `I` is an interface type (v1.4) claims only that the
  **interface value** is non-nil, never that its dynamic value is. The
  notation and `Type.NonNil` representation are identical to concrete `!T`;
  the guarantee differs by the kind of Go value. See
  [docs/rfc-interface-semantics.md](rfc-interface-semantics.md). No schema
  bump — `!` on interface types was already valid syntax.
- `"![]T"`, `"!map[K]V"`, `"!chan T"`, `"!func(…)"` (v1.6) claim only that
  the **reference value** (slice header, map, channel, function value) is
  non-nil — never a length, non-emptiness, or channel-state contract. A
  leading `!` binds **only the outermost type constructor**:
  `"!map[string]*T"` says the map is non-nil and nothing about the `*T`
  values.
- `"[]!*T"`, `"map[string]!V"`, `"[8]!*Slot"` (v1.7 / M2b) — an `!` in
  element / value position is legal and denotes an element contract. The
  recorded flag still reflects only the outermost `!`; element nilability
  is documentation across the `.gna` boundary and is enforced only at
  local composite-literal construction sites. An `!` on a **channel
  element** (`"chan !*T"`) or a **map key** (`"map[!*K]V"`) remains a load
  error. See
  [docs/rfc-element-contract-construction.md](rfc-element-contract-construction.md).
  No schema bump.
- `"!T"` where `T`'s underlying kind is **not** nilable (struct, array,
  basic) is a malformed contract — **GN003** (v1.6).

## Rules

1. `schema: 1` is required. An unknown schema version is a hard error.
2. One package ↔ one `.gna` file. Splitting is forbidden.
3. Duplicate function or method keys in the same file → error.
4. An annotation that names a `functions:` / `methods:` / `types:` symbol
   absent from the real package → **GW004** (warning). Since **v1.5** the
   checker validates this against `go/types` for every resolved package;
   `methods:` resolution uses the full method set, so a method promoted
   from an embedded field is accepted. (A future strict/CI mode may
   promote GW004 to an error.)
5. If a package is imported but has no `.gna` → every member is treated
   as ordinary. Missing annotation is **not** an error.
6. Signature shape mismatch (the `params` / `results` count disagrees with
   `go/types`; a variadic final parameter counts as one) → **GN003**
   (error). Since **v1.5** the offending entry is also **dropped**: none of
   its `!` flags are applied, so a stale annotation cannot shift a claim
   onto the wrong argument. Type *strings* after `!` are never compared —
   arity is the only shape property checked.
7. No flow-sensitive semantics.
8. No conditional contracts (“non-nil when err == nil”).
9. `.gna` declares no type parameters or constraints (no syntax, schema stays 1). Generic symbols are annotated like any other and handled by rules 11–12 (v1.9); source-level generic `!` contracts are `docs/rfc-generic-semantics.md` (v1.8).
10. Since **v1.5**, `types:` field contracts are enforced across the
    package boundary at the same three sites as local field contracts —
    construction (`pkg.T{…}` keyed, `new(pkg.T)`, `&pkg.T{}`, `pkg.T{}`;
    one hop into embedded external types), mutation (`x.F = nil`), and use
    (`x.F` is a non-nil source). Unkeyed `pkg.T{a, b}` stays unchecked
    (external field order is not reconstructed). A `!` position whose Go
    type is an interface carries interface-value semantics
    (`docs/rfc-interface-semantics.md`).
11. **Flag validity (v1.9, `docs/rfc-generic-gna.md` X3).** After the arity
    check passes, every `!` flag is validated against the symbol's **origin
    signature** from `go/types` (before any instantiation). A flag is valid
    only if the type at that position is a nilable kind (pointer, slice,
    map, channel, function, interface, `unsafe.Pointer`), or a type
    parameter whose constraint restricts it to nilable kinds (some
    constraint element is a non-empty union of all-nilable terms —
    `rfc-generic-semantics.md` §3). An invalid flag is **GN003** and **only
    that flag is dropped**: the position becomes ordinary and the entry's
    other positions keep their contract (an arity mismatch, rule 6, still
    drops the whole entry). This covers `params`, `results` and `types:`
    fields, for generic and non-generic symbols alike — `"!string"` for a
    `string` parameter is GN003, the rule this spec has stated since v1.6,
    enforced since v1.9. A type parameter declared by a method itself
    (Go 1.27) is validated like any other.
12. **Generic symbols (v1.9).** Type strings may mention type parameters
    (`"![]E"`, `"!S"`); the names are documentation — only the position
    matters, so an upstream rename changes nothing. An explicit
    instantiation (`pkg.F[T](…)`, `pkg.Box[T]{…}`, `new(pkg.Box[T])`)
    resolves to the same entry as the inferred form. `methods:` is keyed by
    the origin type name and resolves through an instantiated receiver;
    `types:` fields apply through instantiation. A `!` **inside a
    type-argument list** of a type string (`"Box[!*T]"`, `"Box[[]!*T]"`) is
    a load error: a type argument carries no contract.

## Authoring principle

> A `.gna` annotation may only strengthen nilability when the annotation
> author has an **explicit contract** guaranteeing it. It must not infer
> non-nilability from implementation details or from “what people usually
> pass”.

In particular, a `[]byte` parameter must not be marked `"![]byte"` merely
because a nil slice is uncommon; a nil slice is a valid Go value for
`[]byte`.

## Out of scope for v1

- Declaring type parameters or constraints in `.gna` (names are documentation; `go/types` is authoritative)
- Variadic special cases beyond ordinary positional matching
- Conditional or flow-dependent nilability
