# RFC: Cross-Package Contracts from Module-Local `.gon` Sources (Gon v1.14 / M10)

**Status:** Draft — owner decisions Q1–Q6 and the amendment of I4 recorded
2026-11-17 (§9, §10). The normative text below is under the owner's review; the
RFC is opened as a PR only after that review. The figures in §2 were measured
on 2026-11-17 (Gon v1.13.0 and a throwaway spike), before any decision.
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

**Normative principle:**

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

## 3. Decision Matrix (as decided, §9)

### C1 — Which imports: the main module only

A package is a source of contracts when its import path is inside the **main
module** (the module whose `go.mod` encloses the package being checked) and its
directory holds at least one non-test `.gon` file. Dependencies in the module
cache are not read, even if they ship `.gon`: their contracts keep coming from
`.gna`. (A `replace` to a local directory is still a dependency.)

### C2 — Never the package being checked

The source resolver serves **only imported** module-local packages. The package
under check never resolves to a source-derived file, in either mode: its own
contracts come from its own source as today (functions and fields since v1.0,
methods through M7). Concretely, the resolver is constructed with the import
path of the package under check (`Self`) and returns "no file" for it. Measured
need: §2.2 (322–327 false GW002 on a package's own symbols without this).

### C3 — What is derived

Every **exported** declaration of the package's non-test `.gon` files, with its
`!` flags read by the preprocessor: functions (params / results), methods by
`Type.Method` (receiver `*` and type parameters stripped), methods of exported
interface types, and exported fields of exported struct types. Unexported
declarations are invisible to importers and are not derived. Receiver
contracts stay out of scope (M7 S8).

Keys follow `gna-spec-v1.md` exactly, so the derived file is indistinguishable
from a hand-written `.gna` for the same contracts:

- a method of a generic type is keyed `Type.Method` without type arguments or
  parameters (`func (b *Box[T]) Put(…)` → `Box.Put`), as M5b keys `.gna`
  methods; a method's own type parameters do not change its key;
- an interface method is keyed `Iface.Method`; an embedded interface's methods
  are derived from the interface that declares them;
- a struct field is keyed by its name; an **embedded** field by its type name
  (as `go/types` names it); a field promoted from an embedded struct is derived
  from the struct that declares it, and the existing `types:` path resolves the
  promotion.

These four cases (generic receivers, receiver type parameters, embedded fields,
interface methods) were **not** proven by the spike (§2, limits); they are
mandatory tests (§7, T3) before the implementation is considered ready.

### C4 — A derived file is complete; GW002 does not apply to it

The derived file represents the **whole exported contract surface** of the
package: every exported function, method, interface method and struct field,
ordinary ones included (with all flags false). It is marked as source-derived,
and **GW002 is never reported for a source-derived file**, even for a symbol the
derivation somehow missed: GW002 means "a hand-written `.gna` forgot this
symbol", which cannot be true of a contract read from the source. (Measured
need: §2.3, 30 false GW002 with an incomplete derivation.) A symbol the
derivation misses is a bug of the derivation, caught by the completeness tests
(§7, T3), not a user-facing warning.

### C5 — Precedence and conflicts with `.gna`: the `.gon` wins, one GW005 per import

When an imported module-local package has `.gon` sources (C1), its contracts
come **only** from them. A `.gna` for the same import path (found by any
resolver of the chain) is **not used**, whatever it says, even where it states a
different `!` for the same symbol. There is no merge.

The shadowed `.gna` is reported as **GW005** (warning): `.gna for <import path>
is ignored: the package has .gon sources, which are authoritative`.

**Unit and determinism of GW005.** The unit is one **import declaration**: the
pair (file being checked, imported path). At most one GW005 is reported per
pair, anchored at that file's import spec, no matter how many selectors, calls
or symbols of the package the file uses, and no matter how many `.gon` files the
imported package has. This is the anchoring M5a uses for GN003 / GW004 on a
`.gna` (per file, at the file's own import). In package mode, two files of the
package under check that both import the path each get their own GW005, at
their own import (as GN003 / GW004 already behave, noted in M8 §14). The order
of diagnostics stays the existing order (files sorted by path, source order
within a file), so a run reports the same GW005 set every time.

GW005 does not depend on the `.gna` being well-formed: a shadowed `.gna` is not
parsed for contracts, so a malformed one is not a GN003.

### C6 — Validation

The derived file goes through the existing validation: GN003 for an arity that
disagrees with `go/types`, GW004 for a symbol the package does not provide. With
the dogfood gate in place these mean the `.gon` and the committed `.go` have
drifted, which is a real error.

### C6b — A `.gon` that cannot be read

If an imported package's `.gon` source cannot be read or parsed (after
preprocessing), the import gets one **GN003** at the import spec
(`malformed source contracts for <import path>: …`) and **no** contract of that
package is applied, as for a malformed `.gna` (I3: never guess). Packages of the
main module that have **no** `.gon` (only `.go` and possibly a `.gna`, such as
Gon's `internal/extlib` fixture) are unaffected: they keep using `.gna`.

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
New diagnostic **GW005** (C5), documented in README and `v1-scope.md` like
GW004 was; Diagnostic Protocol v1 is unchanged (`code` is a free string, so a new
code is not a schema change). No new syntax, `.gna` schema stays 1. Ships
**solo** as v1.14.

### C10 — Non-goals

Flow analysis (I1); inferring contracts from bodies (I2); reading `.gon` from
the module cache (C1); `!` on receivers (M7 S8); changing the immediate-use
promotion rule of v1.1.

## 4. Normative Rules

1. **Main module only** (C1). 2. **Never itself** (C2). 3. **Exported
declarations, by name, as `.gna` would** (C3). 4. **Complete: no GW002** (C4).
5. **One source of truth per package: `.gon` wins, the `.gna` is ignored, one
GW005 per import declaration** (C5). 6. **Existing validation; a malformed
`.gon` is GN003 and applies nothing** (C6, C6b). 7. **Deterministic** (C7).
8. **Packages without `.gon` keep `.gna`** (C6b).

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

- `gna.SourceResolver{Root, ModulePath, Self}`: serves an import path inside
  `ModulePath` whose directory holds non-test `.gon` files, except `Self`
  (C1, C2); reads them in sorted order, preprocesses and parses each, derives a
  complete `gna.File` (C3, C4) marked source-derived; caches it per import path
  for the run (C7).
- In the default chain it is consulted **first** for main-module imports; when
  it serves a path, the directory resolvers are still asked only to learn
  whether a `.gna` exists, so GW005 can be reported (C5); that `.gna` is never
  used.
- The checker suppresses GW002 for source-derived files (C4) and reports GW005
  once per (file, import path) at the import spec (C5), and GN003 at the import
  for an unreadable `.gon` (C6b).
- `CheckPackage` and the file mode pass `Self` (C2, C8).

## 7. Test Requirements

| # | Requirement |
|---|---|
| T1 | An importer sees each kind of exported contract: a `!` result (GW001 on `== nil`), a `!` parameter (GN001 on `nil`), a `!` struct field (GN002 on construction, GN001 on `nil`), a `!` on an interface method (GN001 through the interface). |
| T2 | **Changing the source contract changes the importer's diagnostics:** removing the `!` removes the importer's GW001 / GN001; adding one adds it. |
| T3 | **Completeness (mandatory before readiness):** generic receivers (`func (b *Box[T]) Put(p !*T)`), a method with its own type parameters, embedded fields (an embedded `!` field and a field promoted from an embedded struct), interface methods including an embedded interface. Each: the importer sees the contract, and the derived file lists the symbol. |
| T4 | No GW002 for an ordinary exported symbol of a source-contract package; no GW002 ever on a source-derived file. |
| T5 | **No self-resolution:** the package under check, in file mode and package mode, gets nothing from the source resolver (its own unexported and exported calls produce exactly today's diagnostics). |
| T6 | **`.gna` ignored:** a `.gna` and `.gon` for the same package, **stating different contracts** for the same symbol: only the `.gon` contract applies (both directions: `.gna` claims `!` where `.gon` does not, and the reverse). One GW005 at the import. A malformed shadowed `.gna` gives GW005, not GN003. |
| T7 | **GW005 unit:** one per (file, import path) however many selectors or `.gon` files; two importing files in package mode → two GW005, each at its own import; same set over 15 repeated runs. |
| T8 | A dependency outside the main module that ships `.gon` is not read (its `.gna` is used). A main-module package with only `.go` + `.gna` keeps using the `.gna`. |
| T9 | A malformed imported `.gon` → one GN003 at the import, no contract applied. |
| T10 | File mode and package mode give the same importer diagnostics. |
| T11 | Immediate-use promotion unchanged: the v1.1 tests pass unchanged. |
| T12 | Differential over the three repositories' `.gon` files and the standard-library corpus; dogfood: Gon's own `gon check ./...` shows exactly the intended change (expected: none except where an importer contradicts `gna.DefaultChain` / `gna.NewRegistry`, of which there is none today). |

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

## 9. Owner Decisions (2026-11-17)

All six answered **as proposed**, with these limits stated by the owner:

1. **Q1 — Scope:** the main module only. Dependencies in the module cache keep
   using `.gna`, even if they ship `.gon`.
2. **Q2 — Contents:** all four kinds: functions, methods, interface methods,
   struct fields.
3. **Q3 — Completeness:** a derived file is complete; GW002 does not apply to it,
   including for symbols without a `!`.
4. **Q4 — Conflicts:** the `.gon` is authoritative; a `.gna` for the same package
   is ignored; **GW005 once per import**, never once per conflicting symbol.
5. **Q5 — Modes:** file mode and package mode alike.
6. **Q6 — Release:** M10 / v1.14, solo, possibly breaking, documented.

Three design conditions the owner attached: the source resolver never reads the
package under check (C2); the derivation represents the whole contract surface
(C4); GW005 is deterministic and not repeated, with a defined unit (C5).
Open evidence the owner requires before readiness: generic receivers, receiver
type parameters, embedded fields and interface methods (T3), and a test that a
`.gna` stating a **different** contract is really ignored (T6).

## 10. Invariant Check (I1–I7)

- **I1** upheld (no flow). **I2** upheld (only written `!`). **I3** upheld
  (matching through `go/types`; a malformed source applies nothing).
- **I4 — amended (owner, 2026-11-17).** New wording:
  > Nilability contracts for external packages come from `.gna`. For an
  > imported module-local package, contracts may come from that package's
  > `.gon` sources; when `.gon` sources are available, they take precedence
  > over a `.gna` for the same package.

  This is not an implicit extension to dependencies that happen to ship
  `.gon`: Q1 still limits it to the main module.
- **I5** upheld. **I6** upheld. **I7** upheld (no syntax; `.gna` schema 1; one
  new warning code, GW005).
