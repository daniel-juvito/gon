package fmt

import (
	"bytes"
	"fmt"
	goformat "go/format"
	"sort"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Format returns formatted Gon source (v1.3 / M6a).
func Format(filename string, src []byte) ([]byte, error) {
	result := preproc.Process(filename, src)
	if result == nil || result.Clean == nil {
		return nil, fmt.Errorf("preprocess failed")
	}
	formatted, err := goformat.Source(result.Clean)
	if err != nil {
		return nil, fmt.Errorf("format clean source: %w", err)
	}
	var offsets []int
	for off := range result.NonNilOffsets {
		offsets = append(offsets, off)
	}
	sort.Ints(offsets)
	if len(offsets) == 0 {
		return formatted, nil
	}
	snippets := make([]string, 0, len(offsets))
	clean := result.Clean
	for _, off := range offsets {
		if off < 0 || off >= len(clean) {
			continue
		}
		end := off
		for end < len(clean) {
			r := clean[end]
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
				r == '_' || r == '*' || r == '.' || r == '[' || r == ']' || r == '(' || r == ')' {
				end++
				continue
			}
			break
		}
		if end > off {
			snippets = append(snippets, string(clean[off:end]))
		}
	}
	out := make([]byte, 0, len(formatted)+len(snippets))
	rest := formatted
	for _, snip := range snippets {
		idx := bytes.Index(rest, []byte(snip))
		if idx < 0 {
			out = append(out, rest...)
			rest = nil
			break
		}
		out = append(out, rest[:idx]...)
		out = append(out, '!')
		out = append(out, rest[idx:idx+len(snip)]...)
		rest = rest[idx+len(snip):]
	}
	out = append(out, rest...)
	return out, nil
}
