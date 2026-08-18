package checker

import "strings"

// ContractTrace is first-class diagnostic data (v1.3 / M2a), not a formatting
// side-effect. It records the static path from a construction site to a
// required non-nil field so GN001/GN002 (and later LSP) can surface contract
// context without changing the semantic result (code + severity + primary
// message stay stable for v1.2 compatibility).
//
// Trace is optional: existing diagnostics without a construction site leave
// it nil.
type ContractTrace struct {
	// Origin describes the construction site, e.g. "S{}", "new(S)", "var S".
	Origin string
	// Path is the field path from the constructed value, e.g. ["Inner", "DB"].
	Path []string
	// Declared is the declared non-nil type at the leaf, when known (e.g. "!*int").
	Declared string
}

// FieldPath joins Path with dots. Empty path yields "".
func (t *ContractTrace) FieldPath() string {
	if t == nil || len(t.Path) == 0 {
		return ""
	}
	return strings.Join(t.Path, ".")
}

// Clone returns a shallow copy with its own Path slice.
func (t *ContractTrace) Clone() *ContractTrace {
	if t == nil {
		return nil
	}
	path := make([]string, len(t.Path))
	copy(path, t.Path)
	return &ContractTrace{
		Origin:   t.Origin,
		Path:     path,
		Declared: t.Declared,
	}
}

// withPath returns a clone with an additional path segment appended.
func (t *ContractTrace) withPath(seg string) *ContractTrace {
	if t == nil {
		return &ContractTrace{Path: []string{seg}}
	}
	c := t.Clone()
	c.Path = append(c.Path, seg)
	return c
}
