# Gon Scope and Guarantees

This document is the contract for what Gon **does** and **does not**
promise. It exists so future contributors do not "improve" the checker by
adding inference that silently expands scope.

Architectural rule (unchanged across v1.x):

> `go/types` determines what a symbol is;  
> `.gna` determines what nilability contract Gon assumes about it.

## Version map

| Version | What landed |
|---------|-------------|
| v1.0.0 | Local non-flow-sensitive checks; `.gna` param/result *declaration*; external packages |
| v1.1.0 | Annotated result positions become **non-nil sources** at immediate use sites |
| v1.2.0 | Field contracts: `!T` field is a storage invariant; construction GN002, mutation GN001, selector non-nil source; `.gna` `types:` |

`.gna` schema remains **1**. v1.1 and v1.2 are checker-semantics changes, not format breaks.

## Guaranteed

Static checks only. Enforced at `gon check` / `gon vet` time.

| Case | Diagnostic | Since |
|------|------------|-------|
| Literal `nil` assigned to a `!T` variable | GN001 | v1.0 |
| Literal `nil` passed to a `!T` parameter (local or `.gna`-annotated) | GN001 | v1.0 |
| Literal `nil` returned from a function with a `!T` result | GN001 | v1.0 |
| Struct / composite construction leaving a required `!T` field at zero | GN002 | v1.0 |
| Literal `nil` assigned into a required `!T` field in a composite literal | GN001 | v1.0 |
| Annotated external-package function or method parameter (`!T` in `.gna`) receiving literal `nil` | GN001 | v1.0 |
| Comparison of a known non-nil name with `nil` | GW001 (warning) | v1.0 |
| Package has `.gna` but the called symbol is not listed | GW002 (warning) | v1.0 |
| Malformed or schema-mismatched `.gna` while resolving a package | GN003 (error) | v1.0 |
| Annotated `!T` result position used in a short declaration / untyped `var` is a non-nil source (GW001 on nil comparison) | GW001 | **v1.1** |
| Construction of a value that leaves a nested / embedded / array-contained `!T` field at zero (zero-value containment walk) | GN002 | **v1.2** |
| Explicit `nil` assigned to a `!T` field (`s.Client = nil`) | GN001 | **v1.2** |
| Selector of a `!T` field is a non-nil source (GW001 on nil comparison; assign/arg to `!U`) | GW001 | **v1.2** |
| External type field contracts via `.gna` `types:` | GN002 / GN001 | **v1.2** |

Warnings alone do **not** fail the process (exit 0). Errors do (exit 1).

### Return-value contracts (v1.1)

An explicitly annotated result position (`"!T"` in `.gna` or `!T` in a local
Gon signature) becomes a **non-nil source** at its **immediate use site**:

```go
// .gna: MustLoad results: ["!*Config"]
cfg := config.MustLoad()   // cfg is a non-nil source
if cfg == nil {}           // GW001

var c = config.MustLoad()  // same (no explicit type)

f, err := config.Open()    // only f (result[0]) is a non-nil source if annotated
```

**Precedence:**

1. Explicit type in the binding wins (`var c !*Config = call()` → non-nil from type).
2. Otherwise, an annotated call result may promote the binding.
3. Conversion, type assertion, selector, index, and plain assignment from an
   existing name do **not** propagate source-ness.

Spec: [docs/rfc-return-value-contracts.md](rfc-return-value-contracts.md).

### Field contracts (v1.2)

A field declared `!T` (in a local Gon type or via `.gna` `types:`) is an
**invariant of its storage location**, enforced only at three local sites:

1. **Construction → GN002.** Forms whose initialization is locally observable
   (`var s S`, `S{}`, composite literals, fixed arrays of structs, nested
   embedding). The checker walks the type under the **zero-value containment**
   rule: a nested type is traversed when the containing type's zero value
   contains an actual value of that nested type. Traversal stops at every
   indirection boundary (`*T`, `[]T`, `map`, `chan`, `interface`, `func`).

2. **Mutation → GN001.** Direct assignment of explicit `nil` into a `!T` field
   (`s.Client = nil`). Ordinary (unannotated) values remain accepted under the
   same conservative rule as other `!T` destinations.

3. **Selector → non-nil source.** Selecting a `!T` field yields a non-nil
   source consumable in immediate contexts (assignment to `!U`, argument to
   `!U` parameter, method receiver). No further propagation.

```go
type Config struct {
    Client !*http.Client
}

var c Config                      // GN002 — Client at zero
_ = Config{}                      // GN002
cfg := Config{Client: http.DefaultClient} // OK
cfg.Client = nil                  // GN001
if cfg.Client == nil {}           // GW001 — selector is non-nil source
```

**Non-sites (no GN002):** assignment from an existing value (`b := a`),
range-loop copies, conversion. The checker does not reconstruct object state
from control flow.

External packages use `.gna`:

```yaml
types:
  Config:
    fields:
      Client: "!*http.Client"
```

Spec: [docs/rfc-field-contracts.md](rfc-field-contracts.md).

## Explicitly not guaranteed

| Area | Why |
|------|-----|
| Ordinary (unannotated) return into `!T` | Still accepted — no flow analysis; monotonic with v1.0 |
| Values held in variables after assignment from another name | No flow-sensitive propagation |
| Conditional contracts (`non-nil when err == nil`) | No path-sensitive analysis |
| Runtime enforcement | Annotations are stripped; emitted Go has ordinary types |
| Automatic inference from implementation bodies | `.gna` and source `!T` are explicit contracts only |
| Generics / type-parameter contracts | Deferred |
| Remote annotation registries | Deferred |
| Flow-sensitive field tracking after construction | Local sites only (construction, mutation, use) |

Examples that remain **accepted** (no diagnostic):

```go
func get() *int { return nil }

var x !*int = get()   // allowed — get() is not annotated !T
var y !*int = other   // allowed — not flow-sensitive

cfg := config.MustLoad()
converted := (*Special)(cfg)
if converted == nil {}   // no GW001 — conversion does not propagate

s.Client = ordinary      // allowed — ordinary into !T field (conservative)
```

## Design principles

1. **Strengthen only.** A `.gna` annotation may claim non-nil only when the
   API has an explicit contract. Do not infer from "usually non-nil" or from
   current implementation details.
2. **Missing annotation is ordinary.** No `.gna` → every member treated as
   ordinary. Not an error.
3. **Broken annotation is an error.** Malformed YAML / wrong schema → hard stop.
4. **Checker does not know where annotations come from.** Resolver
   (local dir, module-relative, future registry) is injected; checker only
   sees contracts.
5. **No second type system.** `go/types` remains the sole authority for Go
   semantics. `.gna` only supplies nilability metadata by position.
6. **No new inference (v1.1 / v1.2).** Only explicit contracts at immediate
   local sites (result positions; field construction / mutation / selector).

## CLI contract

```
gon check|vet <file.gon>     # diagnostics → stderr; exit 1 on errors
gon transpile <file.gon>     # write <file>.go; diagnostics → stderr
gon build <file.gon>         # transpile then go build
gon version
gon help
```

| Exit | Meaning |
|------|---------|
| 0 | success (warnings alone do not fail) |
| 1 | checker error, Go type error, or build failure |
| 2 | usage / invalid arguments |

Diagnostic format:

```text
file.gon:12:7: error GN001: cannot pass nil as non-nil argument 1 to demo.Take
file.gon:12:7: warning GW001: x is non-nil; comparison with nil is always false
```

Positions refer to the **Gon source** (`.gon`), not generated `.go`.

## After v1.2

Candidate topics for later RFCs (not part of this release):

- Generic / type-parameter nilability
- Optional strict mode (ordinary value into `!T` becomes an error)
- Community / remote annotation registry

Do not land flow analysis or body inference under a v1.x tag without an RFC.
