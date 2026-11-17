// Package transpiler emits clean Go source from preprocessed Gon source.
// Since preproc already stripped all ! modifiers, the clean source is
// valid Go. We just format it.
package transpiler

import (
	"go/format"
	"os"
)

// Transpile formats clean Go source with go/format, in process.
//
// Until v1.11 this ran the `gofmt` found on PATH and, when that failed,
// silently returned the unformatted source, so the same .gon could emit
// different bytes on different machines. go/format is the same formatter
// (identical output on the repository's own sources) and does not depend on
// PATH. Source that does not parse is an error rather than being passed through.
func Transpile(cleanSrc []byte) ([]byte, error) {
	return format.Source(cleanSrc)
}

// TranspileToFile formats and writes clean Go source to outPath.
func TranspileToFile(cleanSrc []byte, outPath string) error {
	out, err := Transpile(cleanSrc)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0644)
}
