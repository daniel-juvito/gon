# Gon

Go with non-nil type annotations.

Gon adds `!T` type modifiers to ordinary Go so you can express and enforce non-nil contracts at compile time (vet time). The toolchain strips the annotations and emits clean Go.

## Install

```bash
go install github.com/daniel-juvito/gon/cmd/gon@latest
```

Or build from source:

```bash
go build -o gon ./cmd/gon
```

## Usage

```bash
gon vet file.gon          # check for nil-safety violations
gon transpile file.gon    # emit clean Go source (file.go)
```

## Syntax

Prefix a type with `!` to mark it non-nil:

```go
var x !*int = &n          // x must not be nil
func f(p !*S) !*int       // parameter and return are non-nil
type S struct { X !*int } // field is required non-nil
func (r !*T) M()          // receiver is non-nil
```

## Diagnostics

| Code  | Severity | Meaning |
|-------|----------|---------|
| GN001 | error    | literal `nil` assigned to / passed as / returned as a non-nil type |
| GN002 | error    | struct literal missing a required non-nil field |
| GW001 | warning  | comparison of a non-nil name with `nil` (always true/false) |

v1.0 performs only local (non-flow-sensitive) checks. Assignments from other expressions are accepted without analysis.

## Status

Phase 1 / v1.0 — covers named parameters, returns, variables, struct fields, and receivers. Unnamed parameters and some multi-result forms are deferred.
