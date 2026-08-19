package fmt

import (
	"fmt"
	goformat "go/format"
	"sort"

	"github.com/daniel-juvito/gon/internal/preproc"
)

// Format returns formatted Gon source (v1.3 / M6a).
//
// Pipeline: preprocess (strip !) → go/format on clean Go → re-insert !
// at non-nil type sites. Re-insertion is sequential and word-boundary
// gated so short type names (S, T, int) cannot match inside larger
// identifiers.
func Format(filename string, src []byte) ([]byte, error) {
	result := preproc.Process(filename, src)
	if result == nil || result.Clean == nil {
		return nil, fmt.Errorf("preprocess failed")
	}
	formatted, err := goformat.Source(result.Clean)
	if err != nil {
		return nil, fmt.Errorf("format clean source: %w", err)
	}
	return reinsertBangs(result.Clean, result.NonNilOffsets, formatted), nil
}

// reinsertBangs walks NonNilOffsets in ascending order, extracts each
// type snippet from the pre-format clean source, then finds the next
// word-boundary match of that snippet in the formatted source and
// prefixes it with '!'.
func reinsertBangs(clean []byte, nonNil map[int]bool, formatted []byte) []byte {
	if len(nonNil) == 0 {
		return formatted
	}
	offsets := make([]int, 0, len(nonNil))
	for off := range nonNil {
		offsets = append(offsets, off)
	}
	sort.Ints(offsets)

	out := make([]byte, 0, len(formatted)+len(offsets))
	rest := formatted
	for _, off := range offsets {
		snip := typeSnippet(clean, off)
		if len(snip) == 0 {
			continue
		}
		idx := indexWordBoundary(rest, snip)
		if idx < 0 {
			// Snippet not found (should be rare after go/format). Preserve
			// remaining source without further re-insertion rather than
			// corrupt an unrelated region.
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
	return out
}

// typeSnippet returns the type text starting at clean[off], stopping at
// the first byte that cannot appear in a Go type expression's prefix
// (identifiers, pointers, selectors, brackets, parens).
func typeSnippet(clean []byte, off int) []byte {
	if off < 0 || off >= len(clean) {
		return nil
	}
	end := off
	for end < len(clean) && isTypeChar(clean[end]) {
		end++
	}
	if end == off {
		return nil
	}
	return clean[off:end]
}

func isTypeChar(r byte) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '*' || r == '.' ||
		r == '[' || r == ']' || r == '(' || r == ')'
}

func isIdentChar(r byte) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}

// indexWordBoundary finds the first occurrence of snip in haystack such
// that it is not a proper substring of a larger identifier:
//   - the byte immediately before the match is not an identifier character
//   - the byte immediately after the match is not an identifier character
//
// This prevents snippet "S" from matching inside "Something" and snippet
// "int" from matching inside "print"/"println".
func indexWordBoundary(haystack, snip []byte) int {
	if len(snip) == 0 {
		return -1
	}
	start := 0
	for start <= len(haystack)-len(snip) {
		i := indexFrom(haystack, snip, start)
		if i < 0 {
			return -1
		}
		beforeOK := i == 0 || !isIdentChar(haystack[i-1])
		after := i + len(snip)
		afterOK := after >= len(haystack) || !isIdentChar(haystack[after])
		if beforeOK && afterOK {
			return i
		}
		start = i + 1
	}
	return -1
}

func indexFrom(haystack, needle []byte, start int) int {
	if start < 0 {
		start = 0
	}
	if start > len(haystack) {
		return -1
	}
	h := haystack[start:]
	for i := 0; i+len(needle) <= len(h); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if h[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return start + i
		}
	}
	return -1
}
