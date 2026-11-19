package checker

import "path/filepath"

// operandName is the file name MalformedConstraint gives every operand. go/build
// decides a file by its name before it reads it: a name the go tool ignores (a
// leading `_` or `.`) or one with a GOOS/GOARCH suffix that does not match the
// platform (`a_windows.go`) is dropped without its //go:build line ever being
// parsed, while `go vet a_windows.go` still rejects that line. A neutral name
// makes the verdict depend on the contents only.
const operandName = "gon_operand.go"

// MalformedConstraint reports whether go/build cannot evaluate the //go:build
// line of src, a .gon file's clean Go form, and if so go/build's reason without
// the file name ("parsing //go:build line: missing close paren"). It is the
// verdict on one file operand (file mode); it never says a file is
// excluded: only an error from go/build counts, and (false, nil) from
// MatchFile, a valid constraint that does not match, is not one. It reads src,
// not the file system, and needs no module.
func MalformedConstraint(src []byte) (string, bool) {
	path := filepath.Join(".", operandName)
	_, err := overlayContext(map[string][]byte{path: src}).MatchFile(".", operandName)
	if err != nil {
		return reasonOf(operandName, err), true
	}
	return "", false
}
