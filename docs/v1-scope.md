# Gon v1.0 — Scope and Guarantees

This document is the contract for what Gon v1.0 **does** and **does not**
promise. It exists so future contributors do not “improve” the checker by
adding inference that silently expands scope.

Architectural rule (unchanged):

> `go/types` determines what a symbol is;  
> `.gna` determines what nilability contract Gon assumes about it.

## Guaranteed in v1.0

Static checks only. Enforced at `gon check` / `gon vet` time.

| Case | Diagnostic |
|------|------------|
| Literal `nil` assigned to a `!T` variable | GN001 |
| Literal `nil` passed to a `!T` parameter (local or `.gna`-annotated) | GN001 |
| Literal `nil` returned from a function with a `!T` result | GN001 |
| Struct literal missing a required `!T` field, or assigning literal `nil` to one | GN001 / GN002 |
| Annotated external-package function or method parameter (`!T` in `.gna`) receiving literal `nil` | GN001 |
| Comparison of a known non-nil name with `nil` | GW001 (warning) |
| Package has `.gna` but the called symbol is not listed | GW002 (warning) |
| Malformed or schema-mismatched `.gna` while resolving a package | GN003 (error) |

Warnings alone do **not** fail the process (exit 0). Errors do (exit 1).

## Explicitly not guaranteed

These are **out of scope** for v1.0. Code that relies on them will not be
diagnosed, and that is intentional.

| Area | Why |
|------|-----|
| Values returned by functions | No return-value inference; `var x !*T = getT()` is allowed |
| Values held in variables after assignment | No flow-sensitive propagation |
| Conditional contracts (`non-nil when err == nil`) | No path-sensitive analysis |
| Runtime enforcement | Annotations are stripped; emitted Go has ordinary types |
| Automatic inference from implementation bodies | `.gna` and source `!T` are explicit contracts only |
| Generics / type-parameter contracts | Deferred |
| Remote annotation registries | Deferred |

Examples that are **accepted** in v1.0 (no diagnostic):

```go
func get() *int { return nil }

var x !*int = get()   // allowed — not flow-sensitive
var y !*int = other   // allowed — not flow-sensitive

func f(w io.Writer) {
    w.Write(nil)      // allowed — io.gna does not claim ![]byte
}
```

## Design principles

1. **Strengthen only.** A `.gna` annotation may claim non-nil only when the
   API has an explicit contract. Do not infer from “usually non-nil” or from
   current implementation details.
2. **Missing annotation is ordinary.** No `.gna` → every member treated as
   ordinary. Not an error.
3. **Broken annotation is an error.** Malformed YAML / wrong schema → hard stop.
4. **Checker does not know where annotations come from.** Resolver
   (local dir, module-relative, future registry) is injected; checker only
   sees contracts.
5. **No second type system.** `go/types` remains the sole authority for Go
   semantics. `.gna` only supplies nilability metadata by position.

## CLI contract (v1.0)

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

## After v1.0

Candidate topics for a separate v1.1 RFC (not part of this release):

- Return-value contracts in `.gna`
- Field annotations under a `types:` section
- Generic / type-parameter nilability
- Optional strict mode (treat GW002 as error)
- Community / remote annotation registry

Do not land inference or flow analysis under the v1.0 tag.
