package checker

import "fmt"

// Severity indicates whether a diagnostic is an error or a warning.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

// Diagnostic is a single error or warning produced by the Gon checker.
type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	File     string
	Line     int
	Col      int
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
