package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// An ordinary flow into a `!T` slot of a non-interface nilable kind is GN001, an
// error (RFC explicit non-nil conversion, sections 6.4 and 6.6, amendment A2 of
// 2026-11-22; it was the warning GW006 in v1.21). These tests fix the final output
// of the CLI: the exit code and the Protocol v1 JSON, not only the presence of a
// code (plan WP4).

type flowDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source"`
	Range    struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"range"`
}

func flowJSON(t *testing.T, stdout string) []flowDiag {
	t.Helper()
	var env struct {
		SchemaVersion int        `json:"schemaVersion"`
		Diagnostics   []flowDiag `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not Protocol v1 JSON: %v\n%s", err, stdout)
	}
	if env.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", env.SchemaVersion)
	}
	return env.Diagnostics
}

const flowSrc = "package main\n\n" +
	"type T struct{}\n\n" +
	"func take(p !*T) {}\n\n" +
	"func main() {\n" +
	"\tq := &T{}\n" +
	"\tvar v *T = q\n" +
	"\ttake(v)\n" +
	"\ttake((!*T)(v))\n" +
	"}\n"

// An ordinary flow alone: exit 1, the JSON diagnostic is an error GN001 with a position at
// the value, and the explicit conversion on the next line is silent.
func TestOrdinaryFlowIsAnErrorAndExitsOne(t *testing.T) {
	root := scModule(t, "1.21", map[string]string{"main.gon": flowSrc})
	chdir(t, root)

	exit, stdout, _ := captureRun([]string{"check", "--json", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (an ordinary flow into !T is an error)\n%s", exit, stdout)
	}
	diags := flowJSON(t, stdout)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1 (the conversion is silent): %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != "GN001" || d.Severity != "error" {
		t.Fatalf("got %s %s, want GN001 error", d.Code, d.Severity)
	}
	if d.Range.Start.Line != 9 {
		t.Errorf("line = %d, want 9 (0-based line of `take(v)`)", d.Range.Start.Line)
	}
	if !strings.Contains(d.Message, "cannot use value of type *T as !*T") ||
		!strings.Contains(d.Message, "use (!*T)(x) to make an explicit claim") {
		t.Errorf("message = %q", d.Message)
	}

	// The same through the human output.
	exit, _, errOut := captureRun([]string{"check", "./..."})
	if exit != 1 || !strings.Contains(errOut, "error GN001:") || strings.Contains(errOut, "GW006") {
		t.Fatalf("human output: exit %d\n%s", exit, errOut)
	}
}

// Mixed with a nil literal both are GN001 errors (two diagnostics, two messages), and
// GW006 is no longer emitted.
func TestOrdinaryFlowMixedWithNilIsTwoErrors(t *testing.T) {
	src := strings.Replace(flowSrc, "\ttake((!*T)(v))\n", "\ttake(nil)\n", 1)
	root := scModule(t, "1.21", map[string]string{"main.gon": src})
	chdir(t, root)

	exit, stdout, _ := captureRun([]string{"check", "--json", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, stdout)
	}
	diags := flowJSON(t, stdout)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %+v, want two GN001 errors", diags)
	}
	var ordinary, nilLit int
	for _, d := range diags {
		if d.Code != "GN001" || d.Severity != "error" {
			t.Fatalf("got %s %s, want GN001 error", d.Code, d.Severity)
		}
		if strings.Contains(d.Message, "cannot use value of type *T as !*T") {
			ordinary++
		}
		if strings.Contains(d.Message, "cannot pass nil") {
			nilLit++
		}
	}
	if ordinary != 1 || nilLit != 1 {
		t.Fatalf("messages = %+v, want one ordinary flow and one nil literal", diags)
	}
}

// An interface slot is not an ordinary-flow case of the non-interface kinds: an ordinary interface value stays GN001
// (D3b) and a concrete operand stays silent (D3a).
func TestOrdinaryFlowInterfaceSlotStaysGN001(t *testing.T) {
	src := "package main\n\n" +
		"type R interface{ M() }\n" +
		"type C struct{}\n\n" +
		"func (C) M() {}\n\n" +
		"func take(r !R) {}\n\n" +
		"func f(r R, c C) {\n" +
		"\ttake(r)\n" +
		"\ttake(c)\n" +
		"\ttake((!R)(r))\n" +
		"}\n\n" +
		"func main() {}\n"
	root := scModule(t, "1.21", map[string]string{"main.gon": src})
	chdir(t, root)
	exit, stdout, _ := captureRun([]string{"check", "--json", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1 (D3b)", exit)
	}
	diags := flowJSON(t, stdout)
	if len(diags) != 1 || diags[0].Code != "GN001" {
		t.Fatalf("diagnostics = %+v, want one GN001", diags)
	}
}

// `.gna` slots are part of the matrix: a parameter, a field, and a result that is
// a source. The package has no .gon sources, so its contracts come from the .gna.
func TestOrdinaryFlowGnaSlots(t *testing.T) {
	gna := "schema: 1\npackage: ex/lib\n" +
		"functions:\n" +
		"  Take:\n    params: [\"!*int\"]\n    results: []\n" +
		"  Open:\n    params: []\n    results: [\"!*int\"]\n" +
		"types:\n  Conn:\n    fields:\n      P: \"!*int\"\n"
	lib := "package lib\n\n" +
		"func Take(p *int) {}\n\n" +
		"func Open() *int { return new(int) }\n\n" +
		"type Conn struct{ P *int }\n"
	app := "package main\n\nimport \"ex/lib\"\n\n" +
		"func main() {\n" +
		"\tx := new(int)\n" + // plain local in Part A
		"\tvar y *int = x\n" +
		"\tlib.Take(y)\n" + // GN001: argument
		"\tlib.Take((!*int)(y))\n" + // conversion: silent
		"\tlib.Take(new(int))\n" + // construction form: silent
		"\tr := lib.Open()\n" + // .gna result: a source
		"\tlib.Take(r)\n" + // silent
		"\t_ = lib.Conn{P: y}\n" + // GN001: field
		"\t_ = lib.Conn{P: r}\n" + // silent
		"\tz := 1\n" +
		"\tlib.Take(&z)\n" + // &operand: silent
		"\t_ = lib.Conn{P: &z}\n" + // silent
		"}\n"
	root := scModule(t, "1.21", map[string]string{
		"annotations/ex/lib.gna": gna,
		"lib/lib.go":             lib,
		"app/main.gon":           app,
	})
	chdir(t, root)
	exit, _, errOut := captureRun([]string{"check", "./app"})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1\n%s", exit, errOut)
	}
	wantDiags(t, errOut, "app/main.gon:8 GN001", "app/main.gon:13 GN001")
}

// A method expression takes the receiver as its first argument, so in
// `(*R).M(f())` result 0 of f is the receiver and result 1 is the first
// parameter. The error must name the type of the result that fills the
// parameter, not the receiver.
func TestOrdinaryFlowTupleIntoMethodExpression(t *testing.T) {
	src := "package main\n\n" +
		"type Receiver struct{}\n\n" +
		"type Node struct{}\n\n" +
		"func pair() (*Receiver, *Node) { return &Receiver{}, &Node{} }\n\n" +
		"func (r *Receiver) M(p !*Node) {}\n\n" +
		"func main() {\n" +
		"\t(*Receiver).M(pair())\n" +
		"}\n"
	root := scModule(t, "1.21", map[string]string{"main.gon": src})
	chdir(t, root)
	exit, stdout, _ := captureRun([]string{"check", "--json", "./..."})
	if exit != 1 {
		t.Fatalf("exit = %d, want 1", exit)
	}
	diags := flowJSON(t, stdout)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %+v, want exactly one", diags)
	}
	if !strings.Contains(diags[0].Message, "cannot use value of type *Node as !*Node") {
		t.Errorf("message = %q, want the type of result 1 (*Node), not of the receiver", diags[0].Message)
	}
}
