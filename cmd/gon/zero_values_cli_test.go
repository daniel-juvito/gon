package main

import "testing"

// Rules A and B of the RFC on local backdoors across the files of one package: the
// array type is declared in one file and its zero value is made in another, through a
// chain of defined types, and a slice chain is made with a length that is not 0.
func TestZeroValuesAcrossFilesOfAPackage(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{
		"types.gon": "package main\n\n" +
			"type T struct{}\n" +
			"type Slots [4]!*T\n" +
			"type More Slots\n" +
			"type Ptrs []!*T\n" +
			"type Many Ptrs\n" +
			"type Holder struct{ S More }\n",
		"use.gon": "package main\n\n" +
			"func main() {\n" +
			"\tvar a More\n" + // line 4: GN002
			"\t_ = new(More)\n" + // line 5: GN002
			"\tvar h Holder\n" + // line 6: GN002
			"\t_ = make(Many, 3)\n" + // line 7: GN002
			"\t_ = make(Many, 0, 3)\n" + // fine
			"\t_, _ = a, h\n" +
			"}\n",
	})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"check", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, errOut)
	}
	wantDiags(t, errOut, "use.gon:4 GN002", "use.gon:5 GN002", "use.gon:6 GN002", "use.gon:7 GN002")
}
