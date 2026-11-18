# Diagnostic Protocol v1

**Status:** Draft (locked decisions)  
**Target consumers:** `gon check --json`, CI, `gon-vscode`, future `gonls`  
**Schema version:** 1 (independent of Gon CLI version)

This document is the machine-readable contract for Gon diagnostics.
The Gon compiler remains the source of truth; tooling is a consumer of this protocol.

## 1. Status & Scope

### In scope for v1

- Structured JSON output of `gon check --json`
- Diagnostic object shape
- Position and range semantics
- File path semantics
- Related locations
- Exit-code contract
- Minimal source enumeration
- Conformance expectations

### Explicitly out of scope for v1

- Incremental / on-type checking
- Full LSP protocol
- Code actions / quick fixes
- Diagnostic categories, help URLs, or fix suggestions
- Flow-sensitive or path-sensitive analysis
- UTF-16 or rune-based columns
- Changes to the existing human-readable diagnostic format

Later protocol versions may add fields; v1 consumers must ignore unknown fields.

## 2. JSON Envelope

When `gon check --json` succeeds in producing protocol output, stdout is a single JSON object:

```json
{
  "schemaVersion": 1,
  "diagnostics": []
}
```

| Field           | Type    | Required | Notes                                      |
|-----------------|---------|----------|--------------------------------------------|
| `schemaVersion` | integer | yes      | Always `1` for this protocol               |
| `diagnostics`   | array   | yes      | Ordered list of Diagnostic objects; may be empty |

- No other top-level fields are required.
- Additional top-level fields may appear in future versions; consumers must ignore them.
- `schemaVersion` is the versioning contract for the JSON shape. It is independent of the Gon CLI version string.

## 3. Diagnostic

Every element of `diagnostics` is an object with the following fields:

| Field                | Type     | Required | Notes |
|----------------------|----------|----------|-------|
| `code`               | string   | **yes**  | e.g. `"GN001"`, `"GW001"`. Never omitted. |
| `severity`           | string   | **yes**  | Exactly `"error"` or `"warning"` |
| `message`            | string   | **yes**  | Human-readable text |
| `source`             | string   | **yes**  | Enumerated; see below |
| `file`               | string   | **yes**  | Absolute, normalized filesystem path |
| `range`              | object   | **yes**  | See Position & Range |
| `relatedInformation` | array    | no       | See Related Locations |

No other fields are defined in v1. Consumers must ignore unknown fields.

In particular, v1 does **not** include top-level `line`, `col`, `length`, or any legacy position fields outside `range`.

### Severity

Only two values are permitted:

- `"error"`
- `"warning"`

There is no `"info"`, `"hint"`, or numeric severity.

### Source

`source` is an enumerated string, not free-form text. For protocol v1 implementations the only permitted values are:

| Value       | Meaning                          |
|-------------|----------------------------------|
| `gon-check` | Produced by the Gon checker      |
| `gon-vet`   | Alias path / historical synonym  |

Implementations of v1 must not emit arbitrary internal strings as `source`.

New source values may be added in later protocol versions. Old consumers must treat unknown `source` values as opaque strings and must not fail solely because of an unrecognized source.

### Code

`code` is a required property of every Diagnostic object.
It is never optional, even when the process later exits with code 3.

Exit 3 is a process/checker failure, not a Diagnostic. See Exit Codes.

## 4. Position & Range Semantics

```json
"range": {
  "start": { "line": 0, "column": 0 },
  "end":   { "line": 0, "column": 5 }
}
```

| Component | Semantics                                      |
|-----------|------------------------------------------------|
| `line`    | Zero-based                                     |
| `column`  | Zero-based **UTF-8 byte offset** within the line |
| `start`   | Inclusive                                      |
| `end`     | Exclusive                                      |

### Important consequences

- Columns are **not** UTF-16 code units.
- Columns are **not** rune (Unicode code point) counts.
- Columns are **not** 1-based.

Example:

```
😀abc
```

The character `a` begins at:

| Measure        | Value |
|----------------|-------|
| UTF-8 bytes    | 4     |
| Rune count     | 1     |
| UTF-16 units   | 2     |

Under this protocol the column of `a` is **4**.

Adapters (e.g. `gon-vscode`, `gonls`) that speak LSP are responsible for converting to UTF-16 if required by the editor protocol. The Gon protocol itself never uses UTF-16.

## 5. File Path Semantics

`file` is:

- an **absolute** path
- **normalized** (as produced by the compiler / OS path utilities)
- a filesystem path

Symlinks are **not** automatically resolved to their physical target.
If the user (or a test) passes a path that is a symlink, that path is what appears in `file`.

Consumers that need a path relative to a workspace root must compute the relative path themselves from the absolute path and their own root(s). This is deliberate: it supports multi-root VS Code workspaces without the compiler having to guess the correct root.

## 6. Related Locations

`relatedInformation` is optional. When present it is an array of objects:

```json
{
  "message": "non-nil contract declared here",
  "location": {
    "file": "/absolute/path/to/config.gna",
    "range": {
      "start": { "line": 4, "column": 8 },
      "end":   { "line": 4, "column": 16 }
    }
  }
}
```

Each related location carries its own `file` and `range`.
There is no implicit inheritance from the primary diagnostic.

This enables cross-file diagnostics, for example:

```
main.gon
   ↓
GN001 (or similar)
   ↓
config.gna   (relatedInformation pointing at the contract declaration)
```

`ContractTrace` data already present in the checker may be projected into relatedInformation in a future iteration; v1 does not require that projection.

## 7. Exit Codes

| Exit | Meaning                                      | JSON envelope |
|------|----------------------------------------------|---------------|
| 0    | Check completed; no contract error and no Go type-checking error | Produced; `diagnostics` may contain only warnings or be empty |
| 1    | Check completed; at least one contract error diagnostic **or** a Go type-checking error | Produced when emission is reached; `diagnostics` contains the contract diagnostics Gon produced, which may be empty (see below). The one exit 1 with no envelope is the file-mode pre-check parse failure |
| 2    | Invocation / configuration / input error, including a package that cannot be loaded | May be absent (checker never entered protocol production) |
| 3    | Internal / compiler failure                  | Not required; consumers must not rely on diagnostics when exit = 3 |

#### Exit status and `diagnostics` are related, not equivalent

The `diagnostics` array holds the **contract diagnostics Gon produced**
(GN001, GN002, GN003 and the GW warnings). A **Go type-checking error** (a Go
parse, type or package error reported by the Go toolchain for source that was
checked; see "Failures, envelope and exit code" below) is **not** an entry of
that array: it has no Diagnostic representation in v1 and is reported on
stderr. Consequently:

- Exit 1 with a contract error in `diagnostics` is the usual case.
- Exit 1 with `diagnostics` empty, or holding only warnings, is **valid**: the
  failure was a Go type-checking error, explained on stderr.
- A contract error diagnostic always implies exit 1; exit 1 does **not** imply
  that `diagnostics` holds an error.
- Exit 0 means neither a contract error nor a Go type-checking error was found.

#### Failures, envelope and exit code

Three kinds of failure that can all look like "it did not parse" or "it did not
load" behave differently:

| Situation | Envelope | Exit |
|-----------|----------|------|
| File-mode pre-check parse failure | No | 1 |
| Package-mode Go parse / type / package error reported by the loader for a package it loaded | Yes | 1 |
| Package-loading, environment or configuration failure (no package was loaded) | No | 2 |
| File mode: a `go.mod` is found above the file but its module cannot be loaded (v1.17) | No | 2 |

- **File-mode pre-check parse failure.** For a **file** operand, the checker
  parses the preprocessed (clean Go) source with the Go parser before any
  contract check runs. If that fails, the run stops with `gon: parse error: …`
  on stderr and exit 1, **before** Protocol JSON emission, so stdout contains
  **no envelope**. (The parser is Go's, applied to Gon's preprocessed output; it
  is Gon's own checker step, not the type-check step.) Whether this should
  instead be an exit 2 or a Diagnostic is a separate decision and is not made
  here.
- **Package-mode loader errors.** For a **directory** or `...` operand there is
  no per-file pre-check: the Go package loader parses and type-checks the
  package. A parse error it reports (a syntax error, or an invalid `package`
  clause), a type error, or a package error (for example two package names in
  one directory) is a Go type-checking error of a checked package: the
  envelope is written with the contract diagnostics of the files that could be
  checked, the exit code is 1, and stderr carries the loader's message. That
  stderr line is labelled `gon: Go type error:` for parse errors too, and its
  position names the overlaid `<base>.go` path, not the `.gon` file.
- **File mode and the file's module (v1.17).** For a **file** operand with a
  `go.mod` above it, Gon loads that module once, before the file is checked,
  and type-checks the file in it. If the module cannot be loaded, the run is
  exit 2 with **no envelope**, nothing is checked (a contract error in the same
  file does not change that) and `gon transpile` / `gon build` write nothing:
  - when golang.org/x/mod shows the `go.mod` is invalid (it does not parse, has
    no `module` directive, or its module path is not a valid import path):
    `gon: invalid go.mod: <reason>`;
  - for any other reason (the `go.mod` needs a newer Go than the toolchain, a
    broken `go.work`, inconsistent vendoring, no `go` command, invalid
    `GOFLAGS`): `gon: cannot load the module of <file>: <the go command's
    error as it is>`; no cause is inferred.

  A `go.mod` the go command accepts is never rejected. Only when **no**
  `go.mod` is found above the file does file mode type-check it with the
  GOROOT/GOPATH importer, as before; until v1.16 it also fell back to that
  importer, silently, when a `go.mod` was found but its module could not be
  loaded.
- **Package-loading failures.** When no package could be loaded at all, the run
  is exit 2 with **no envelope** and the reason on stderr: the `go` command is
  missing or fails, the module configuration is invalid (for example an
  unparsable `go.mod`), no source of an explicitly named package is in the
  build (`build constraints exclude all Go files in <dir> (.gon files:
  <files>)`, every file named, sorted), or the loader returned no type
  information. These are not Go type-checking errors. The same applies to the
  directory form of `gon transpile` and `gon build`.
- **Build constraints decide package membership (v1.16).** Inside a module the
  go command decides which files belong to a package for the current build
  configuration, as `go list ./dir` does: a `.gon` excluded by build
  constraints (a `//go:build` line, a file name suffix, a tag in `GOFLAGS`) is
  not part of the package and is **not checked**; it is neither a diagnostic
  nor a failure. A package whose `.gon` files are all excluded but which has
  other Go files in the build is checked from those. A package found by a
  `...` pattern none of whose sources is in the build is skipped (a pattern
  all of whose packages are skipped warns on stderr and is exit 0 with an
  empty envelope); named explicitly, it is the package-loading failure above.
  (In v1.15 any excluded `.gon` was exit 2, except, inconsistently, a
  directory whose only `.gon` was excluded, which was checked as if it were
  not.) **Outside a module (v1.18)** there is no go command package for a
  directory: go/build decides membership with the default build context of
  the toolchain that built Gon (GOOS, GOARCH, release tags; tags in `GOFLAGS`
  are **not** applied there), every member `.gon` and plain `.go` file is
  loaded as one package, and the same rules follow (an excluded `.gon` is not
  checked; no source in the build is exit 2 when named, skipped under
  `...`). Imports outside a module resolve as before (the standard library
  only).

Consumers must not assume that every exit 1 produces parseable JSON: the
file-mode pre-check parse failure is the exception, and stderr explains it. It
is the only exit 1 without an envelope (audited for v1.15: every exit-1 path of
`gon check` / `gon vet`).

A consumer that needs the reason for an exit 1 with no error diagnostic reads
stderr; it must not treat that combination as malformed output or as an exit
code 3. This rule applies to every way of invoking the checker, not to one mode.

#### All or nothing

One invocation writes **one** envelope holding the diagnostics of **every**
operand, or none. When an operand fails before emission (a file-mode pre-check
parse failure, exit 1, or a package that cannot be loaded, exit 2), no envelope
is written and the diagnostics already collected from earlier operands are
**not** sent, in `--json` mode or anywhere else. A consumer that receives an
envelope therefore has the complete result of the invocation; Protocol v1 has
no partial result.

Examples:

| Situation                     | Exit |
|-------------------------------|------|
| Clean file                    | 0    |
| Warning only (e.g. GW001)     | 0    |
| Go type-checking error only (`diagnostics` empty) | 1 |
| GN001 / GN002 / GN003         | 1    |
| File-mode pre-check parse failure (no envelope) | 1 |
| Package-mode syntax error or invalid `package` clause (envelope) | 1 |
| Non-existent input file       | 2    |
| Malformed CLI invocation      | 2    |
| Package cannot be loaded (`go` missing, invalid `go.mod`, no source of a named package in the build) | 2 |
| File mode, the file's `go.mod` found but its module cannot be loaded | 2 |
| A `.gon` excluded by build constraints (not checked) | as the rest of the package |
| Controlled internal failure   | 3    |

Warnings alone never produce exit 1.

### Internal failure injection

There is no public CLI flag that forces exit 3.

Injection is test-only (e.g. environment variable such as `GON_TEST_INJECT_FAILURE` available only in test binaries / test harnesses). It is not part of the documented public API and must not become a compatibility surface.

## 8. Compatibility

- `schemaVersion` is the sole versioning signal for the JSON shape.
- Adding a new optional field inside Diagnostic or the envelope is a non-breaking change.
- Changing the meaning of an existing field, removing a required field, or altering position/range/path semantics is a breaking change and requires a new schemaVersion.
- New `source` values are non-breaking for well-behaved consumers.
- Exit-code meanings are part of the contract; they must not be reassigned.
- Clarification (v1.10): §7 states explicitly that exit 1 also covers a Go
  type-checking error, and that exit status and the `diagnostics` array are
  related but not equivalent. This is a clarification of semantics already
  implied by the human output, not a new exit code and not a change of JSON
  shape; `schemaVersion` stays `1`.

### Relationship to human-readable output

`gon check` (without `--json`) continues to emit the classic human-readable form:

```
file:line:column: severity CODE: message
```

That format is **not** governed by this protocol. Position numbers in the human form remain 1-based (and `File` may remain basename-only) for compatibility with existing tools and editor click-to-source behaviour.

The JSON path is a separate serialization / adaptation layer:

```
existing checker diagnostic
        |
        +-- human formatter → existing behavior (unchanged)
        |
        +-- JSON formatter  → Protocol v1
```

Implementations must not refactor the human path solely to make JSON easier.

## 9. Conformance Tests

Fixtures live under:

```
testdata/diagnostic-protocol/
├── clean/
├── errors/
├── warnings/
├── positions/
│   └── utf8/
├── paths/
│   └── symlink/
├── related/
│   └── cross-file/
├── multiple-files/
├── invocation/
└── internal-failure/
```

### Required coverage (minimum)

| Concern                         | Fixture / test area   |
|---------------------------------|-----------------------|
| Clean file → empty diagnostics, exit 0 | `clean/`         |
| Semantic error (GN001 / GN002)  | `errors/`             |
| Warning-only (GW001)            | `warnings/`           |
| Non-existent / malformed input  | `invocation/`         |
| Controlled internal failure     | `internal-failure/` (harness-driven) |
| UTF-8 / emoji before diagnostic position | `positions/utf8/` |
| Symlink path not implicitly resolved | `paths/symlink/` |
| Multiple input files            | `multiple-files/`     |
| Package mode (v1.10): one document, many files | unit tests (`cmd/gon/package_check_test.go`) |
| Optional `relatedInformation` schema round-trip | unit test (no producer) |

`related/cross-file/` is a **reserved** fixture area for a future producer that
projects `ContractTrace` (or equivalent) into `relatedInformation`. It is
**not** required coverage for protocol v1: the producer projection is deferred
(see §6). v1 still requires that the **schema** for `relatedInformation` be
round-trip tested (construct → `json.Marshal` → `json.Unmarshal` → assert
`message`, `location.file`, `location.range`).

### Assertions that must hold for protocol output

- `schemaVersion == 1`
- Every Diagnostic has required fields: `code`, `severity`, `message`, `source`, `file`, `range`
- `severity` ∈ {`"error"`, `"warning"`}
- `source` ∈ {`"gon-check"`, `"gon-vet"`} for v1 implementations
- `line` and `column` are zero-based
- `range.end` is exclusive
- `column` is a UTF-8 byte offset
- `file` is absolute and normalized
- Symlink paths appear as the path that was supplied, not the resolved physical path
- Exit 0 / 1 / 2 / 3 behave as specified above
- When `relatedInformation` is present on a Diagnostic, each entry has
  `message` and `location` with `file` and `range` (schema round-trip; no
  producer required in v1)

### Test matrix (codes)

- GN001 JSON
- GN002 JSON
- GW001 JSON
- (and other stable codes as they appear)

---

## Appendix A — Minimal examples

### Clean

```json
{
  "schemaVersion": 1,
  "diagnostics": []
}
```
Exit: 0

### Single error

```json
{
  "schemaVersion": 1,
  "diagnostics": [
    {
      "code": "GN001",
      "severity": "error",
      "message": "cannot assign nil to non-nil type !*int",
      "source": "gon-check",
      "file": "/abs/path/to/main.gon",
      "range": {
        "start": { "line": 2, "column": 10 },
        "end":   { "line": 2, "column": 13 }
      }
    }
  ]
}
```
Exit: 1

### Warning only

```json
{
  "schemaVersion": 1,
  "diagnostics": [
    {
      "code": "GW001",
      "severity": "warning",
      "message": "comparison of non-nil value with nil is always false",
      "source": "gon-check",
      "file": "/abs/path/to/main.gon",
      "range": {
        "start": { "line": 4, "column": 4 },
        "end":   { "line": 4, "column": 14 }
      }
    }
  ]
}
```
Exit: 0

### UTF-8 position illustration

Source line (bytes):

```
😀abc
```

`😀` occupies bytes 0–3; `a` starts at byte 4.
A diagnostic pointing at `a` reports `"column": 4`.

A conformance fixture places multi-byte content on the same line, before the token that is diagnosed, so the column of that token is a non-trivial UTF-8 byte offset.

---

## Appendix B — Implementation note (serialization layer)

The existing checker continues to produce internal diagnostics with whatever position representation it already uses (currently 1-based, basename file for human output).

The `--json` path is responsible for:

1. Mapping internal diagnostics into the Protocol v1 shape.
2. Converting positions to zero-based UTF-8 byte offsets.
3. Emitting absolute, normalized paths.
4. Restricting `source` to the v1 enumeration.
5. Never altering the human formatter path.
