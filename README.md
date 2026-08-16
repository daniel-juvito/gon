# Gon

Go with non-nil type annotations.

Gon adds `!T` type modifiers to ordinary Go so you can express and enforce
non-nil contracts at **vet time**. The toolchain strips the annotations and
emits clean Go that `go build` accepts.

**v1.0 is local and non-flow-sensitive.** It does not track values across
assignments or infer nilability from implementation details.

`!T` is a **static Gon guarantee** enforced only for the cases covered by the
v1 checker (literal `nil` into `!T` slots, required struct fields, and
explicit `.gna` contracts). It is not a runtime non-nil guarantee.

Architectural rule:

> `go/types` determines what a symbol is;  
> `.gna` determines what nilability contract Gon assumes about it.

Full scope contract: [docs/v1-scope.md](docs/v1-scope.md)

## Install

```bash
go install github.com/daniel-juvito/gon/cmd/gon@v1.0.0
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

# emit clean Go
gon transpile file.gon    # writes file.go

# transpile then go build
gon build file.gon
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
type S struct { X !*int } // field is required non-nil
func (r !*T) M()          // receiver is non-nil
```

Because v1 is not flow-sensitive, non-literal assignments are accepted:

```go
func get() *int { return nil }

var x !*int = get()   // allowed in v1 — not tracked
var y !*int = other   // allowed in v1 — not tracked
```

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
- Unknown symbol in an annotated package → `GW002`
- Annotations may only **strengthen** nilability under a real API contract

Full format: [docs/gna-spec-v1.md](docs/gna-spec-v1.md)

## Diagnostics

| Code  | Severity | Meaning |
|-------|----------|---------|
| GN001 | error    | literal `nil` assigned / passed / returned where `!T` is required |
| GN002 | error    | struct literal missing a required non-nil field |
| GN003 | error    | malformed or mismatched `.gna` while resolving a package |
| GW001 | warning  | comparison of a non-nil name with `nil` (always true/false) |
| GW002 | warning  | package has `.gna` but this symbol is not listed |

## v1.0 scope and limitations

**In scope**

- Named parameters, results, variables, struct fields, receivers
- Package functions and methods (value, pointer, interface, method expression)
- External packages via `.gna` + module-aware `go/packages`
- Local, non-flow-sensitive checks only

**Out of scope (v1)**

- Flow-sensitive analysis / assignment propagation
- Return-value inference
- Automatic nilability inference from bodies
- Runtime enforcement (annotations are stripped)
- Generic type contracts
- Remote annotation registries

These are intentional. See [docs/v1-scope.md](docs/v1-scope.md) for the
authoritative list of guarantees and non-guarantees.

## Development

```bash
go test ./...
go vet ./...
```

Module path: `github.com/daniel-juvito/gon`
