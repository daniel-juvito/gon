// Package transpiler emits clean Go source from preprocessed Gon source.
// Since preproc already stripped all ! modifiers, the clean source is
// valid Go. We just run it through gofmt.
package transpiler

import (
	"bytes"
	"os"
	"os/exec"
)

// Transpile formats clean Go source using gofmt.
// If gofmt is unavailable or fails, the clean source is returned as-is.
func Transpile(cleanSrc []byte) ([]byte, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = bytes.NewReader(cleanSrc)
	out, err := cmd.Output()
	if err != nil {
		return cleanSrc, nil
	}
	return out, nil
}

// TranspileToFile formats and writes clean Go source to outPath.
func TranspileToFile(cleanSrc []byte, outPath string) error {
	out, err := Transpile(cleanSrc)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0644)
}
