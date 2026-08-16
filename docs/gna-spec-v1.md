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

## Rules

1. `schema: 1` is required. An unknown schema version is a hard error.
2. One package ↔ one `.gna` file. Splitting is forbidden.
3. Duplicate function or method keys in the same file → error.
4. An annotation that names a function/method absent from the real
   package → warning (or error under a strict/CI mode).
5. If a package is imported but has no `.gna` → every member is treated
   as ordinary. Missing annotation is **not** an error.
6. Signature shape mismatch (wrong number of params/results compared
   with `go/types`) → annotation conflict error.
7. No flow-sensitive semantics.
8. No conditional contracts (“non-nil when err == nil”).
9. No generic / type-parameter support in v1.

## Authoring principle

> A `.gna` annotation may only strengthen nilability when the annotation
> author has an **explicit contract** guaranteeing it. It must not infer
> non-nilability from implementation details or from “what people usually
> pass”.

In particular, a `[]byte` parameter must not be marked `"![]byte"` merely
because a nil slice is uncommon; a nil slice is a valid Go value for
`[]byte`.

## Out of scope for v1

- Generics / type parameters
- Variadic special cases beyond ordinary positional matching
- Struct field annotations (may appear later under a `types:` section)
- Conditional or flow-dependent nilability
