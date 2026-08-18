package checker

import "fmt"

// Severity indicates whether a diagnostic is an error or a warning.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

// Diagnostic is a single error or warning produced by the Gon checker.
//
// Trace is optional structured contract context (v1.3). String() keeps the
// v1.2 wire format so existing CLI and tests remain stable; consumers that
// need the chain (LSP, golden tests) read Trace directly.
type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	File     string
	Line     int
	Col      int
	// Trace is nil for diagnostics that are not construction-site derived.
	Trace *ContractTrace
}

func (d *Diagnostic) String() string {
	level := "error"
	if d.Severity == SeverityWarning {
		level = "warning"
	}
	return fmt.Sprintf("%s:%d:%d: %s %s: %s",
		d.File, d.Line, d.Col, level, d.Code, d.Message)
}

func (d *Diagnostic) IsError() bool {
	return d.Severity == SeverityError
}
