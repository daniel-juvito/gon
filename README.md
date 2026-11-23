# Gon

Go with non-nil type annotations.

Gon adds `!T` type modifiers to ordinary Go so you can express and enforce
non-nil contracts at **vet time**. The toolchain strips the annotations and
emits clean Go that `go build` accepts.

**v1.x is local and non-flow-sensitive.** It does not track values across
assignments or infer nilability from implementation details.

`!T` is a **static Gon guarantee** enforced only for the cases covered by the
checker (literal `nil` into `!T` slots, required struct fields, explicit
`.gna` contracts, annotated return positions as non-nil sources, field storage
invariants, complete local-struct construction including `new(T)` and unkeyed
literals since v1.3, since v1.4 interface value contracts (`!I` means the
interface value is non-nil; the dynamic value is never constrained), and —
since v1.5 — external `.gna` `types:` field contracts enforced across the
package boundary plus `.gna` validation against `go/types` (arity → GN003,
unknown symbol → GW004), since v1.6 a uniform meaning for `!` on
every nilable kind (`![]T` / `!map` / `!chan` / `!func` and named types /
aliases): the reference value is non-nil, nothing more, since v1.7
element contracts (`[]!*T`, `[N]!*T`, `map[K]!*V`, `[]!I`) checked at the
composite-literal construction site, and — since v1.8 — generics: contracts
are attached to declaration positions and survive instantiation (`Box[T]{}`,
`NN[T]()`), `!X` is valid on a type parameter only when its constraint
restricts it to nilable kinds, and `!` in a type argument or constraint is
rejected; since v1.9 the same holds for external generics described in `.gna`
(explicit and inferred instantiation resolve alike, and every `.gna` `!` flag
is validated against the symbol's real signature). It is not a runtime
non-nil guarantee.

Architectural rule:

> `go/types` determines what a symbol is;  
> `.gna` determines what nilability contract Gon assumes about it.

Full scope contract: [docs/v1-scope.md](docs/v1-scope.md)

## What each version added

One line per minor version, newest first; patch releases are folded into the line of their minor version. The detail, with the cases and the exact messages, is in [CHANGELOG.md](CHANGELOG.md), and a few versions changed an exit code, a diagnostic or what is accepted, which can matter to a script or a CI job.

| Version | What it added or changed |
|---------|--------------------------|
| [v1.22](CHANGELOG.md#1220--2026-11-22) | An ordinary value of a pointer, slice, map, channel or function kind flowing into a `!T` slot is now **GN001, an error (exit 1)**; in v1.21 it was the warning GW006, which is no longer emitted. `(!T)(x)` is the only way through. A possibly breaking change, shipped alone. |
| [v1.21](CHANGELOG.md#1210--2026-11-22) | The explicit non-nil conversion `(!T)(x)` (an author's claim, no run-time check) and the new warning **GW006**: a value that Gon cannot see to be non-nil flowing into a `!T` slot of a pointer, slice, map, channel or function kind now warns (exit code unchanged), and `&operand`, `&T{}`, `new`, `make`, literals, a `!T` result and `(!T)(x)` are recognised. `!!T` is `!T`. `var x ! *T` is now enforced (it was silently ignored): **possibly breaking**, for a CI that fails on warnings and for code that relied on that. |
| [v1.20](CHANGELOG.md#1200--2026-11-21) | File mode and `gon build <dir>`: a malformed `//go:build` in a file operand is now **exit 1** (it was exit 0), and `gon build <dir>` with no module for the operand is **exit 2** before anything is written (it was exit 1 after writing the `.go` files). |
| [v1.19](CHANGELOG.md#1190--2026-11-19) | Package mode: a malformed `//go:build` line is a source error of its package (the file is not checked, the others are, **exit 1**; it was exit 2), and names the go tool ignores are not part of the package. |
| [v1.18](CHANGELOG.md#1180--2026-11-19) | Package mode outside a module: a directory with several `.gon` files is checked as one package (it was **exit 2**), and build constraints apply as inside a module. |
| [v1.17](CHANGELOG.md#1170--2026-11-18) | File mode: when a `go.mod` is found but its module cannot be loaded, the run is **exit 2** with nothing checked or written (it used to fall back silently to the GOROOT/GOPATH importer). |
| [v1.16](CHANGELOG.md#1160--2026-11-18) | Package discovery follows the go command: a `.gon` excluded by build constraints is not part of the package; `./...` stops at a nested module; `transpile`/`build` write only the files that were checked. |
| [v1.15](CHANGELOG.md#1150--2026-11-17) | Exit codes of package loading follow Diagnostic Protocol v1 section 7: a package that cannot be loaded (no `go` command, invalid `go.mod`, a file excluded by build constraints) is **exit 2**, no longer exit 1. |
| [v1.14](CHANGELOG.md#1140--2026-11-17) | Cross-package contracts from module-local `.gon` sources: a `!` on an exported declaration reaches the packages that import it, with no `.gna` to write for your own packages; adds GW005. Possibly breaking for an importer that contradicts an exported `!`. |
| [v1.13](CHANGELOG.md#1130--2026-11-17) | The operands of a `return` are checked like any other expression, and a function literal is checked as a function (both were gaps). Possibly breaking for code that was silently accepted. |
| [v1.12](CHANGELOG.md#1120--2026-11-16) | Directory form for `gon transpile` and `gon build`: a directory (or `./...`) is checked as one package and the `.go` files are written only if it passes; formatting runs in process with `go/format`. Additive. |
| [v1.11](CHANGELOG.md#1110--2026-11-16) | Call-site contracts for source-declared methods: a `!` on a method's parameter or result is enforced at its callers (through `go/types` object identity). Possibly breaking for code that was silently accepted. |
| [v1.10](CHANGELOG.md#1100--2026-11-15) | Package-aware checking: `gon check` / `gon vet` accept a directory or `./...` and check it as one package; contracts are shared across its files. Additive, except that `--json` on a file with a Go type error now exits 1. |
| [v1.9](CHANGELOG.md#190--2026-11-14) | Generic `.gna`: explicit instantiation resolves like inference, and every `.gna` `!` flag is validated against the symbol's signature (an invalid flag is GN003 and only that flag is dropped). Possibly breaking, by design. Patches: 1.9.1 fixed release binaries failing outside a module (GOROOT); 1.9.2 fixed a method overwriting the contract of a same-named function. |
| [v1.8](CHANGELOG.md#180--2026-11-14) | Generic semantics: contracts attach to declaration positions and survive instantiation; `!X` is valid on a type parameter only when its constraint restricts it to nilable kinds (otherwise GN003). Possibly breaking. Patch 1.8.1: the build minimum is Go 1.27.0 (generic methods). |
| [v1.7](CHANGELOG.md#170--2026-09-07) | Element contracts: `[]!*T`, `[N]!*T`, `map[K]!*V`, `[]!I` are checked at the construction site (a construction-time obligation, not a lifetime invariant). Non-breaking. Patch 1.7.1: untyped local `var x = <expr>` initializers are now checked. |
| [v1.6](CHANGELOG.md#160--2026-09-06) | Type coverage: `!` has one meaning on every nilable kind (`![]T`, `!map`, `!chan`, `!func`, named types); `var x !S` with no initializer is GN002 and `!` on a non-nilable type is GN003. Adds new rejections compared with v1.5. |
| [v1.5](CHANGELOG.md#150--2026-09-06) | Ecosystem contracts: external `.gna` `types:` field contracts apply across the package boundary, and a `.gna` is validated against `go/types` (arity: GN003; unknown symbol: GW004). Possibly breaking in three narrow ways that surface defects that were silent. Patch 1.5.1: no more spurious `could not import` for module-local imports. |
| [v1.4](CHANGELOG.md#140--2026-09-05) | Interface contracts: `!I` means the interface value is non-nil, never its dynamic value (D3a to D3d); the only new rejection is an ordinary interface value into `!I` (GN001). Patch 1.4.1: `gon fmt` no longer mis-places `!` when a type appears both as `!T` and `T`. |
| [v1.3](CHANGELOG.md#130--2026-08-19) | Construction completeness (`new(T)`, unkeyed literals), diagnostic traces (`ContractTrace`), GW003, and the tools `gon fmt` and `gon lsp`. |
| [v1.2](CHANGELOG.md#120--2026-08-17) | Field contracts: a `!T` field is an invariant of its storage (GN002 at construction, GN001 on assignment of `nil`, a non-nil source on selection), and the `.gna` `types:` section. Patch 1.2.1: `!` in multi-result function signatures is recognised. |
| [v1.1](CHANGELOG.md#110--2026-08-17) | Return-value contracts: an annotated `!T` result position is a non-nil source at its immediate use site. |
| [v1.0](CHANGELOG.md#100--2026-08-16) | First release: local non-flow-sensitive nilability checks (`!T`), external packages through `.gna` and `go/packages`, and conservative annotations for `io` and `os`. |

## Install

```bash
go install github.com/daniel-juvito/gon/cmd/gon@v1.22.0
```

Or from source:

```bash
git clone https://github.com/daniel-juvito/gon
cd gon
go build -o gon ./cmd/gon
```

### Go versions

- **Release binaries** (GitHub Releases, with `checksums.txt`) need no Go at
  all.
- **Building Gon** (`go install` or from source) needs **Go 1.27.0 or newer**
  (`go.mod`). With Go 1.21+ and the default `GOTOOLCHAIN=auto`, `go install`
  downloads the required toolchain by itself and leaves your installed Go
  untouched; this needs network access and fails with `GOTOOLCHAIN=local` or
  on Go older than 1.21.
- **Checking your code** uses the `go` command to load your module, and
  type-checking follows the `go` directive in *your* `go.mod`. A project that
  declares `go 1.21` cannot use language features from a newer Go; for
  example, methods with their own type parameters
  (`func (b Box[X]) Map[Y any](…)`) need `go 1.27` in your `go.mod`.
- **Which user Go versions are tested.** CI runs a release-style binary with
  your `go` forced to each of **1.21.13, 1.22.12, 1.23.12, 1.24.7, 1.25.0,
  1.26.0 and 1.27.0**, on a file inside a module and on one outside any
  module, and requires identical diagnostics. Older than 1.21 is not tested,
  and neither is a Go newer than the one Gon was built with.

## Quickstart

```bash
# check (alias: vet)
gon check file.gon

# check a directory as ONE package (its .gon files together)
gon check ./internal/foo
gon check ./...           # every directory below here that holds .gon files

# format in place (preserves !)
gon fmt file.gon

# emit clean Go
gon transpile file.gon    # writes file.go

# transpile then go build
gon build file.gon

# a package: check it as a whole, write .go only if it passes (v1.12)
gon transpile ./internal/foo   # or ./... (packages are done after the ones they import)
gon build ./cmd/app            # transpile, then go build ./cmd/app (flags after the target go to go build)

# minimal stdio language server
gon lsp
```

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | success (warnings alone do not fail) |
| 1 | checker error, Go type error, or build failure |
| 2 | usage / invalid arguments |

Diagnostics go to **stderr**. Format:

```text
file.gon:12:5: error GN001: cannot assign nil to non-nil variable !x
file.gon:20:8: warning GW001: x is non-nil; comparison with nil is always false
```

Positions refer to the Gon source (`.gon`), not generated `.go`.

### Checking a package

`gon check file.gon` checks that file **alone**: it is placed in a synthetic
one-file package, so it cannot use a declaration from a sibling file. To check
real multi-file packages, name the **directory**:

```bash
gon check ./internal/foo
```

- The directory's `.gon` files are type-checked **together, where they live**;
  its `.go` files are loaded as they are, and a `.gon` replaces the `.go` of the
  same base name (the one `gon transpile` writes next to it). `*_test.go` and `*_test.gon` files
  are not part of the check.
- A contract written in one file (`!` parameter, field or result) applies in every
  other file of the package.
- Every diagnostic names its own file; `--json` is still one document. A Go type
  error in one file does not stop the others from being checked.
- `./...` (or `dir/...`) checks every directory that holds `.gon` files, skipping
  `testdata`, `vendor`, and names starting with `_` or `.`, like the `go` tool.
- Contracts of *imported* packages still come from `.gna`.
- A folder of independent programs is **not** a package unless you name it as
  one; give those files as file operands.

## Syntax

Prefix a type with `!` to mark it non-nil:

```go
var x !*int = &n          // x must not be literal nil
func f(p !*S) !*int       // parameter and return are non-nil
type S struct { X !*int } // field is required non-nil (storage invariant)
func (r !*T) M()          // receiver is non-nil
```

Because v1.x is not flow-sensitive, ordinary (unannotated) non-literal
assignments into `!T` are accepted:

```go
func get() *int { return nil }

var x !*int = get()   // allowed — get() is not annotated !T
var y !*int = other   // allowed — not flow-sensitive
```

**v1.1 — return-value contracts.** An annotated `!T` result becomes a
non-nil source at the immediate use site:

```go
// .gna results: ["!*Config"]
cfg := config.MustLoad()  // cfg is a non-nil source
if cfg == nil {}          // GW001

// Local multi-return (v1.2.1+ preprocessor)
func Open() (!*string, error)
f, err := Open()          // only f is a non-nil source
```

Conversion and assignment from names do not propagate source-ness.
See [docs/rfc-return-value-contracts.md](docs/rfc-return-value-contracts.md).

**v1.2 — field contracts.** A `!T` field is a storage invariant:

```go
type Config struct {
    Client !*http.Client
}

var c Config                         // GN002 — Client at zero
cfg := Config{Client: http.DefaultClient} // OK
cfg.Client = nil                     // GN001
if cfg.Client == nil {}              // GW001 — selector is non-nil source
```

Structural walk covers embedded structs and fixed arrays; stops at every
indirection (`*T`, `[]T`, `map`, …). External types use `.gna` `types:`.
See [docs/rfc-field-contracts.md](docs/rfc-field-contracts.md).

**v1.3 — construction completeness.** Local-struct construction is fully
checked, including `new(T)` and unkeyed literals:

```go
type Outer struct {
    Name !*string
    In   struct{ X !*int }
}

_ = new(Outer)                 // GN002 — zero-value construction site
_ = Outer{}                    // GN002
n, x := "gon", 1
_ = Outer{&n, struct{ X !*int }{&x}} // OK — unkeyed, declaration order

// Unkeyed external SelectorExpr literals stay behind the firewall;
// keyed pkg.T{…}, new(pkg.T), &pkg.T{} and pkg.T{} are checked (v1.5)
```

Diagnostics may carry a `ContractTrace` (origin path) for tooling; the
printed `String()` wire format is unchanged. Redundant type assertions on
declared `!T` identifiers emit **GW003**.

Tooling:

- `gon fmt` — `go/format` on the clean form, then token-lockstep `!` re-insertion (v1.4.1; refuses rather than write a mis-aligned result)
- `gon lsp` — minimal stdio LSP (`initialize` / `didOpen` / `didChange` /
  diagnostics / `shutdown`); same checker pipeline as `gon check`

Examples: [gon-examples](https://github.com/daniel-juvito/gon-examples)
(`construction/` covers M2a).

**v1.4 — interface value contracts.** `!I` guarantees the *interface value*
is non-nil. It says nothing about the dynamic value, so a typed-nil concrete
value still satisfies `!I`:

```go
var p *File            // nil concrete pointer
var r !io.Reader = p    // OK — the interface value is non-nil (D3a)

var o io.Reader = mk()
var x !io.Reader = o    // GN001 — an ordinary interface value cannot
                        // satisfy !I, even after `if o != nil` (D3b)

var y !io.Reader = nil  // GN001 (D3c)
_ = v.(!io.Reader)      // GN001 — not a type-assertion target (D6a)
```

An existing `!I` binding, or a result declared `!I`, satisfies `!I` — but
only for the *same* interface type. `!` does not propagate across an
embedded/embedding interface: `!ReadCloser` is not a `!Reader` source.
See [docs/rfc-interface-semantics.md](docs/rfc-interface-semantics.md).

## External packages (`.gna`)

Nilability for imported APIs is described in YAML annotation files:

```
<module-root>/annotations/<import-path>.gna
```

Examples: `annotations/io.gna`, `annotations/os.gna`,
`annotations/github.com/acme/lib.gna`.

Rules:

- `"!T"` is an explicit non-nil claim
- `"T"` (or omitted) is ordinary — no non-nil claim
- Missing annotation → ordinary (not an error)
- Malformed annotation → hard error
- Calling an un-annotated symbol of an annotated package → `GW002`
- Annotations may only **strengthen** nilability under a real API contract
- Field contracts for external types live under `types:` (v1.2); since v1.5
  they are enforced at construction, mutation, and use across the boundary
- Since v1.5 the `.gna` is validated against `go/types`: a `params`/`results`
  arity mismatch → `GN003` (and the entry is dropped); a `functions:` /
  `methods:` / `types:` key the package does not provide → `GW004`

Full format: [docs/gna-spec-v1.md](docs/gna-spec-v1.md)

## Diagnostics

| Code  | Severity | Meaning |
|-------|----------|---------|
| GN001 | error    | literal `nil` assigned / passed / returned where `!T` is required; `nil` assigned into a `!T` field (local or external, v1.5); an ordinary interface value where `!I` is required (incl. an external `!I` field, v1.5); `!T` as a type-assertion target — interface (v1.4) or any kind (v1.6); `nil` as an element / map value under an element contract (v1.7); `nil` into a `!` field / argument through a generic instantiation (v1.8), including an explicit instantiation of an external generic (v1.9); **since v1.22:** a value of a pointer, slice, map, channel or function kind that is not a recognised non-nil source (or is one of a different type) flowing into a `!T` slot, which was the warning GW006 in v1.21 — write `(!T)(x)` to claim it |
| GN002 | error    | construction leaves a required non-nil field at zero (including nested / embedded / array; includes `new(T)`; external `pkg.T` incl. one-hop embedded promotion, v1.5); bare `var x !S` of a nilable reference kind with no initializer (v1.6); fixed-array literal leaves a `!` nilable element index at zero (v1.7); construction of an instantiated generic struct leaves a `!` field at zero (v1.8), local or external via `.gna` `types:` (v1.9); `var v !P` with no initializer for a nilable type parameter `P` (v1.8) |
| GN003 | error    | malformed `.gna`, or (v1.5) a `params`/`results` arity mismatch vs `go/types` — the entry is dropped; `!` on a non-nilable type (v1.6); element `!` on a channel element, map key, or non-nilable element kind (v1.7); `!X` on a type parameter not restricted to nilable kinds, and `!` inside a type argument or constraint (v1.8); a `.gna` `!` flag over a non-nilable kind or type parameter — only that flag is dropped (v1.9) |
| GW001 | warning  | comparison of a non-nil name or `!T` field selector (local or external, v1.5) with `nil` (always true/false) |
| GW002 | warning  | calling an un-annotated symbol of an annotated package (never for a package whose contracts come from its `.gon` sources, v1.14) |
| GW003 | warning  | redundant type assertion on a declared `!T` identifier (v1.3) |
| GW004 | warning  | `.gna` names a `functions:` / `methods:` / `types:` symbol the package does not provide (v1.5) |
| GW005 | warning  | a `.gna` exists for an imported package of the main module that has `.gon` sources: the `.gna` is ignored, the `.gon` sources are authoritative (v1.14); once per import |
| GW006 | warning  | (v1.21 only) a value that is not a recognised non-nil source, or is one of a different (not identical) type, flows into a `!T` slot of a pointer, slice, map, channel or function kind. **Since v1.22 the same flow is GN001, an error, and GW006 is no longer emitted**; the explicit conversion `(!T)(x)` is the only way through. An ordinary interface value into `!I` was GN001 already |

### Explicit non-nil conversion (v1.21)

`(!T)(x)` is the author's claim that `x`, converted to `T`, is non-nil. It is valid
exactly when the Go conversion `(T)(x)` is valid, it performs **no run-time check**,
and it is a recognised non-nil source of type `T`. Its first `(` must be a
grouping parenthesis, so `adder(!flag)(3)` keeps its meaning as a call.

```go
func lookup(m map[string]*Node, k string) !*Node {
	n := m[k]            // an ordinary *Node
	return (!*Node)(n)   // the claim; without it: GW006 (v1.21), GN001 (v1.22)
}
```

`(!T)(nil)` is GN001 and a target that is not a pointer, slice, map, channel,
function or interface is GN003. The conversion binds no local variable, and a
contract is never carried to a different type implicitly: `(!U)(x)` is the form
that states the contract on `U`. In a type position a run of `!` is one `!` (`gon fmt`
writes `!T`); `!!x` in an expression is still the Go double negation. See
[docs/rfc-explicit-nonnil-conversion.md](docs/rfc-explicit-nonnil-conversion.md).

## Scope and limitations

**In scope**

- Named parameters, results, variables, struct fields, receivers
- Package functions and methods (value, pointer, interface, method expression)
- A `!` on a method's parameter or result, written in the checked files, is enforced at its call sites (v1.11): matched by `go/types` object identity; a method value carries nothing
- External packages via `.gna` + module-aware `go/packages`
- Local, non-flow-sensitive checks only
- Annotated return positions as non-nil sources at immediate use sites (v1.1)
- Field storage invariants: construction, mutation, selector (v1.2)
- Local-struct construction completeness: `new(T)`, keyed/unkeyed, nested (v1.3)
- `gon fmt` and minimal stdio LSP (v1.3)
- Interface value contracts: `!I` is interface-value non-nil only (v1.4)
- External `.gna` `types:` field contracts across the package boundary, and
  `.gna` validation against `go/types` — GN003 arity, GW004 unknown symbol (v1.5)
- Cross-package contracts from `.gon` sources: an importer sees the `!`
  contracts of an imported package of the **main module** that has `.gon`
  sources, with no `.gna` to write (v1.14); dependencies keep using `.gna`
- Type coverage: `!` on slice / map / chan / func / pointer and named types /
  aliases means the reference value is non-nil (not length / emptiness /
  channel state); bare `var x !S` needs an initializer (v1.6)
- Element contracts: `[]!*T` / `[N]!*T` / `map[K]!*V` / `[]!I` checked at the
  composite-literal construction site — explicit-nil element (GN001) and
  fixed-array element zero-fill (GN002); construction-time only, not a
  lifetime invariant (v1.7). **Known limit:** a contract on a mutable container is not
  carried by its aliases, because `!T` flows into `T` without a diagnostic (a non-nil
  value is a subtype of a nilable one). After `a := []!*T{p}; var b []*T = a`, the
  statements `b[0] = nil` and `a[0] = nil` are accepted, and `a[0]` is then nil. The same
  holds for a map (`map[K]!*V`). Gon checks the
  construction, not every later write; making these containers invariant is a larger
  change, listed in `docs/backlog.md`

**Out of scope**

- Flow-sensitive analysis / assignment propagation
- Conditional contracts (`non-nil when err == nil`)
- Automatic nilability inference from bodies
- Runtime enforcement (annotations are stripped)
- Declaring type parameters or constraints in `.gna` (names are documentation; `go/types` decides)
- A built-in "nilable" constraint (see the roadmap note)
- Interface **dynamic-value** nilability (`!I` never constrains it — by design)
- Cross-package unkeyed / positional construction expansion (M4)
- Remote annotation registries
- LSP hover / completion

These are intentional. See [docs/v1-scope.md](docs/v1-scope.md) for the
authoritative list of guarantees and non-guarantees.

## Development

```bash
go test ./...
go vet ./...
```

Module path: `github.com/daniel-juvito/gon`
