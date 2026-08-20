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
| 0    | Check completed; no error diagnostics        | Produced; `diagnostics` may contain only warnings or be empty |
| 1    | Check completed; at least one error diagnostic | Produced; `diagnostics` contains ≥1 error |
| 2    | Invocation / configuration / input error     | May be absent (checker never entered protocol production) |
| 3    | Internal / compiler failure                  | Not required; consumers must not rely on diagnostics when exit = 3 |

Examples:

| Situation                     | Exit |
|-------------------------------|------|
| Clean file                    | 0    |
| Warning only (e.g. GW001)     | 0    |
| GN001 / GN002 / GN003         | 1    |
| Non-existent input file       | 2    |
| Malformed CLI invocation      | 2    |
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
