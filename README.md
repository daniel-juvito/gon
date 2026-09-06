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
unknown symbol → GW004). It is not a runtime non-nil guarantee.

Architectural rule:

> `go/types` determines what a symbol is;  
> `.gna` determines what nilability contract Gon assumes about it.

Full scope contract: [docs/v1-scope.md](docs/v1-scope.md)

## Install

```bash
go install github.com/daniel-juvito/gon/cmd/gon@v1.5.1
```

Or from source:

```bash
git clone https://github.com/daniel-juvito/gon
cd gon
go build -o gon ./cmd/gon
```

## Quickstart

```bash
# check (alias: vet)
gon check file.gon

# format in place (preserves !)
gon fmt file.gon

# emit clean Go
gon transpile file.gon    # writes file.go

# transpile then go build
gon build file.gon

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
| GN001 | error    | literal `nil` assigned / passed / returned where `!T` is required; `nil` assigned into a `!T` field (local or external, v1.5); an ordinary interface value where `!I` is required (incl. an external `!I` field, v1.5), or `!I` as a type-assertion target (v1.4) |
| GN002 | error    | construction leaves a required non-nil field at zero (including nested / embedded / array; includes `new(T)`; external `pkg.T` incl. one-hop embedded promotion, v1.5) |
| GN003 | error    | malformed `.gna`, or (v1.5) a `params`/`results` arity mismatch vs `go/types` — the entry is dropped |
| GW001 | warning  | comparison of a non-nil name or `!T` field selector (local or external, v1.5) with `nil` (always true/false) |
| GW002 | warning  | calling an un-annotated symbol of an annotated package |
| GW003 | warning  | redundant type assertion on a declared `!T` identifier (v1.3) |
| GW004 | warning  | `.gna` names a `functions:` / `methods:` / `types:` symbol the package does not provide (v1.5) |

## Scope and limitations

**In scope**

- Named parameters, results, variables, struct fields, receivers
- Package functions and methods (value, pointer, interface, method expression)
- External packages via `.gna` + module-aware `go/packages`
- Local, non-flow-sensitive checks only
- Annotated return positions as non-nil sources at immediate use sites (v1.1)
- Field storage invariants: construction, mutation, selector (v1.2)
- Local-struct construction completeness: `new(T)`, keyed/unkeyed, nested (v1.3)
- `gon fmt` and minimal stdio LSP (v1.3)
- Interface value contracts: `!I` is interface-value non-nil only (v1.4)
- External `.gna` `types:` field contracts across the package boundary, and
  `.gna` validation against `go/types` — GN003 arity, GW004 unknown symbol (v1.5)

**Out of scope**

- Flow-sensitive analysis / assignment propagation
- Conditional contracts (`non-nil when err == nil`)
- Automatic nilability inference from bodies
- Runtime enforcement (annotations are stripped)
- Generic type contracts
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
