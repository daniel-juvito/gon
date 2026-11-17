package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// Protocol v1 JSON types — adapter layer only.
// Spec: docs/diagnostic-protocol-v1.md

type protocolEnvelope struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Diagnostics   []protocolDiagnostic `json:"diagnostics"`
}

type protocolDiagnostic struct {
	Code               string            `json:"code"`
	Severity           string            `json:"severity"`
	Message            string            `json:"message"`
	Source             string            `json:"source"`
	File               string            `json:"file"`
	Range              protocolRange     `json:"range"`
	RelatedInformation []protocolRelated `json:"relatedInformation,omitempty"`
}

type protocolRange struct {
	Start protocolPos `json:"start"`
	End   protocolPos `json:"end"`
}

type protocolPos struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type protocolRelated struct {
	Message  string      `json:"message"`
	Location protocolLoc `json:"location"`
}

type protocolLoc struct {
	File  string        `json:"file"`
	Range protocolRange `json:"range"`
}

func protocolFilePath(input string) (string, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func toProtocolDiagnostic(d *checker.Diagnostic, fileAbs string, source string, origSrc []byte) protocolDiagnostic {
	sev := "error"
	if d.Severity == checker.SeverityWarning {
		sev = "warning"
	}

	line0, col0 := mapCleanPosToGon(origSrc, d.Line, d.Col)

	return protocolDiagnostic{
		Code:     d.Code,
		Severity: sev,
		Message:  d.Message,
		Source:   source,
		File:     fileAbs,
		Range: protocolRange{
			Start: protocolPos{Line: line0, Column: col0},
			End:   protocolPos{Line: line0, Column: col0 + 1},
		},
	}
}

func mapCleanPosToGon(origSrc []byte, cleanLine1, cleanCol1 int) (line0, col0 int) {
	if cleanLine1 < 1 {
		cleanLine1 = 1
	}
	if cleanCol1 < 1 {
		cleanCol1 = 1
	}

	fallback := func() (int, int) {
		l, c := cleanLine1-1, cleanCol1-1
		if l < 0 {
			l = 0
		}
		if c < 0 {
			c = 0
		}
		return l, c
	}

	if len(origSrc) == 0 {
		return fallback()
	}

	res := preproc.Process("_", origSrc)
	if len(res.CleanToOrig) == 0 {
		return fallback()
	}

	cleanOff := lineColToOffset(res.Clean, cleanLine1, cleanCol1)
	if cleanOff < 0 || cleanOff >= len(res.CleanToOrig) {
		if len(res.CleanToOrig) == 0 {
			return fallback()
		}
		cleanOff = len(res.CleanToOrig) - 1
	}
	origOff := res.CleanToOrig[cleanOff]
	return offsetToLineCol0(origSrc, origOff)
}

func lineColToOffset(src []byte, line1, col1 int) int {
	line, col := 1, 1
	for i := 0; i < len(src); i++ {
		if line == line1 && col == col1 {
			return i
		}
		if src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return len(src)
}

func offsetToLineCol0(src []byte, off int) (line0, col0 int) {
	if off < 0 {
		off = 0
	}
	if off > len(src) {
		off = len(src)
	}
	line0, col0 = 0, 0
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line0++
			col0 = 0
		} else {
			col0++
		}
	}
	return line0, col0
}

func writeProtocolJSON(diags []protocolDiagnostic) error {
	env := protocolEnvelope{
		SchemaVersion: 1,
		Diagnostics:   diags,
	}
	if env.Diagnostics == nil {
		env.Diagnostics = []protocolDiagnostic{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}

func testInjectFailure() bool {
	return os.Getenv("GON_TEST_INJECT_FAILURE") != ""
}

func protocolSourceFor(cmd string) string {
	switch cmd {
	case "vet":
		return "gon-vet"
	default:
		return "gon-check"
	}
}

func parseCheckArgs(args []string) (jsonOut bool, files []string, err error) {
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		if stringsHasPrefixDash(a) {
			return false, nil, fmt.Errorf("unknown flag: %s", a)
		}
		files = append(files, a)
	}
	return jsonOut, files, nil
}

func stringsHasPrefixDash(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
