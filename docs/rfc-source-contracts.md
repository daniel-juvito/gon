# RFC: Cross-Package Contracts from Module-Local `.gon` Sources (Gon v1.14 / M10)

**Status:** Draft — awaiting owner review. Decisions C1–C10 are *proposals*;
§9 lists the questions that need an explicit owner answer. The figures in §2
were measured on 2026-11-17 (Gon v1.13.0 and a throwaway spike), before any
decision.
**Target:** Gon v1.14
**Related:** `docs/rfc-package-check.md` (M8: Q5 kept imported packages on
`.gna`, and named this RFC as later work), `docs/rfc-method-contracts.md` (M7),
`docs/gna-spec-v1.md`, `docs/dogfood.md` (decision A + C)
**Date:** 2026-11-17

## 1. Summary

A `!` on an exported declaration of a `.gon` package is enforced inside that
package (and, since v1.11, on its methods), but a package that **imports** it
does not see it: under the M8 model an importer reads an imported package's
contracts from `.gna` only. Measured on Gon's own sources after the dogfood
conversion: `gna.DefaultChain` returns `!*Chain`, yet `c := gna.DefaultChain(…);
c == nil` in `cmd/gon` is not a GW001 (it is one inside package `gna`).

This RFC makes a **module-local** package's `.gon` sources a source of contracts
for the packages that import it, without copying them into a `.gna` by hand.

**Normative principle (proposed):**

> For an import inside the main module whose directory holds `.gon` sources, the
> contracts of its exported declarations are read from those sources, by
> declaration, exactly as a `.gna` would state them; the `.gna` machinery
> (resolution through `go/types`, GN003 / GW004 validation) is reused unchanged.

## 2. Measurements (2026-11-17, before any decision)

A throwaway `SourceResolver` was added to the resolver chain of a scratch copy:
for an import path inside the main module it read the package's non-test
`.gon` files, preprocessed them, and built a `gna.File` in memory (functions,
methods by `Type.Method`, struct fields; `!` flags from the preprocessor's
offsets). It was run with package mode over Gon's eight converted packages, plus
one probe in `cmd/gon` (`c := gna.DefaultChain(…); c == nil`).

| Spike mode | Diagnostics | Probe GW001 in the importer | GW002 on the package's **own** symbols | Check time |
|---|---|---|---|---|
| no source resolver (v1.13 behaviour) | 0 | no | 0 | 3.35 s |
| derived file lists only symbols with a `!` | 1 GW001 + 357 GW002 | **yes** | 327 | 3.49 s |
| derived file lists every exported symbol | 1 GW001 + 323 GW002 | **yes** | 322 | 3.65 s |

Reading of the results:

1. **Feasible.** The importer sees the exported `!` result; the existing `.gna`
   path resolves it through `go/types` with no change to the checker's rules.
2. **The package being checked must not resolve to itself.** 322–327 GW002
   were on a package's *own* methods (for example `checker.Checker.addError`):
   a selector on a local type goes to the `.gna` method path, which now found a
   derived file for the package itself and reported every unlisted method. Own
   contracts already come from the source (M7), so the package under check must
   be excluded.
3. **A derived file must be complete, or GW002 must not apply to it.** Listing
   only `!` symbols produced 30 GW002 in importers for ordinary symbols
   (`lsp.New`, `goenv.EnsureGOROOT`, …). GW002 means "this package has a `.gna`
   that forgot this symbol", which is false for a contract derived from source.
4. **Interface methods** are needed too: one GW002 remained for
   `gna.Resolver.Resolve`, an interface method the spike did not derive.
5. **Cost:** +4 % to +9 % check time on this repository (parsing the imported
   packages' `.gon` sources once per check).

Limits: one repository, a spike that skipped interface methods, type
parameters on receivers and embedded fields, and no external module that ships
`.gon`.

## 3. Decision Matrix (proposals)

### C1 — Which imports: the main module only

A package is a source of contracts when its import path is inside the **main
module** (the module whose `go.mod` encloses the package being checked) and its
directory holds at least one non-test `.gon` file. Dependencies in the module
cache are not read, even if they ship `.gon`: their contracts keep coming from
`.gna`. (A `replace` to a local directory is still a dependency.)

### C2 — Never the package being checked

The package under check never resolves to a source-derived file; its own
contracts come from its source as today (M7 for methods). Measured need: §2.2.

### C3 — What is derived

Every **exported** declaration of the package's non-test `.gon` files, with its
`!` flags read by the preprocessor: functions (params / results), methods by
`Type.Method` (receiver `*` and type parameters stripped), methods of exported
interface types, and exported fields of exported struct types. Unexported
declarations are invisible to importers and are not derived. Receiver
contracts stay out of scope (M7 S8).

### C4 — A derived file is complete; GW002 does not apply to it

The derived file lists every exported symbol, ordinary or not, so an importer
never gets GW002 for a symbol of a source-contract package. (Measured need:
§2.3.)

### C5 — Precedence and conflicts with `.gna`

If a `.gna` exists for the same import path **and** the package has `.gon`
sources, the `.gna` is **not used** for it and a **GW005** warning says so once
per import, at the import: one source of truth. (Alternatives in §8.)

### C6 — Validation

The derived file goes through the existing validation: GN003 for an arity that
disagrees with `go/types`, GW004 for a symbol the package does not provide. With
the dogfood gate in place these mean the `.gon` and the committed `.go` have
drifted, which is a real error.

### C7 — Cache and determinism

A derived file is built once per import path per check (the resolver chain
already caches resolved files), from files read in sorted order. The
diagnostics of a run do not depend on directory or map iteration order.

### C8 — Single-file mode too

The same applies in file mode: the importer is the one file; its imports inside
the main module are read the same way.

### C9 — Compatibility

**Possibly breaking (R1):** an importer that contradicts an exported `!` of a
module-local `.gon` package gets GN001 / GN002 / GW001 it did not get before.
New diagnostic **GW005** (C5). No new syntax, `.gna` schema stays 1. Ships
**solo** as v1.14.

### C10 — Non-goals

Flow analysis (I1); inferring contracts from bodies (I2); reading `.gon` from
the module cache (C1); `!` on receivers (M7 S8); changing the immediate-use
promotion rule of v1.1.

## 4. Normative Rules (proposed)

1. **Main module only** (C1). 2. **Never itself** (C2). 3. **Exported
declarations, by name, as `.gna` would** (C3). 4. **Complete: no GW002** (C4).
5. **One source of truth per package; GW005 on a shadowed `.gna`** (C5).
6. **Existing validation** (C6). 7. **Deterministic** (C7).

## 5. Examples

```go
// internal/gna (a .gon package of the main module)
func DefaultChain(wd, startDir string) !*Chain { … }

// cmd/gon (imports it)
c := gna.DefaultChain(".", ".")
if c == nil {}          // GW001 (v1.14): the importer sees the ! result
gna.Process(nil)        // GN001 if Process declared a ! parameter
```

## 6. Implementation Sketch

`gna.SourceResolver{Root, ModulePath, Self}` in the default chain **after** the
directory resolvers, plus the C5 shadowing check; `preproc.Process` and
`go/parser` per file; derivation per C3; GW002 suppressed for files marked as
source-derived (C4). `CheckPackage` and file mode pass `Self`.

## 7. Test Requirements

1. An importer sees an exported `!` result (GW001), a `!` parameter (GN001), a
   `!` field (GN002 / GN001) and a `!` on an interface method.
2. **Changing the source contract changes the importer's diagnostics** (remove
   the `!` → the GW001 disappears).
3. No GW002 for an ordinary exported symbol of a source-contract package; no
   self-resolution (the package's own unexported calls give nothing).
4. A `.gna` and `.gon` for the same package → GW005, `.gna` ignored.
5. A dependency outside the main module with `.gon` is not read.
6. Determinism over repeated runs; file mode and package mode agree.
7. Immediate-use promotion unchanged (the v1.1 tests pass as they are).
8. Differential over the repositories' `.gon` files and the standard-library
   corpus; dogfood: Gon's own `gon check ./...` shows exactly the intended
   change.

## 8. Alternatives Considered

- **Copy contracts into a `.gna` by hand** — rejected in dogfood decision B:
  nothing keeps two copies' flags equal.
- **Generate a `.gna` file from `.gon` and commit it** — a gate could keep it
  in sync, but it is a second artifact for the same fact.
- **`.gna` wins over `.gon`** (instead of C5), or **merge both** — both keep
  two sources of truth.
- **Read contracts from the imported package's syntax via `go/packages`
  overlays** — heavier (re-type-checks dependencies from source) for the same
  information the preprocessor already gives by name.

## 9. Open Questions for the Owner

1. **Q1 — C1 scope.** Main module only (proposed), or also dependencies that
   ship `.gon`?
2. **Q2 — C3 contents.** Functions, methods, interface methods and struct fields
   (proposed), or start with functions and methods only?
3. **Q3 — C4.** Complete derived files with no GW002 (proposed)?
4. **Q4 — C5 conflict policy.** `.gon` wins, `.gna` ignored with a new **GW005**
   (proposed); or `.gna` wins; or an error?
5. **Q5 — C8.** File mode too (proposed), or package mode only?
6. **Q6 — Release shape.** Solo v1.14, possibly breaking (proposed)?

## 10. Invariant Check (I1–I7)

- **I1** upheld (no flow). **I2** upheld (only written `!`). **I3** upheld
  (matching through `go/types`). **I4** *amended*: nilability of an imported
  module-local package may come from its `.gon` sources, not only `.gna`; the
  RFC needs the owner's explicit acceptance of this. **I5** upheld. **I6**
  upheld. **I7** upheld (no syntax; schema 1).
