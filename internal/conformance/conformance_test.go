package conformance

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/daniel-juvito/gon/internal/checker"
	gonfmt "github.com/daniel-juvito/gon/internal/fmt"
	"github.com/daniel-juvito/gon/internal/preproc"
)

// marker is written in an expected clean source immediately before the token
// that carries a `!`. It is an oracle written by hand: the expected offsets are
// not read back from the preprocessor.
const marker = "‹!›"

type entry struct {
	ID         string `json:"id"`
	RFC        string `json:"rfc"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Site       string `json:"site"`
	Diagnostic string `json:"diagnostic"`
	Note       string `json:"note"`
}

func dataDir() string { return filepath.Join("..", "..", "testdata", "conformance") }

func norm(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }

func loadManifest(t *testing.T) []entry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dataDir(), "manifest.json"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var es []entry
	if err := json.Unmarshal([]byte(norm(b)), &es); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return es
}

// loadFixtures returns id -> part ("src", "want") -> text.
func loadFixtures(t *testing.T) map[string]map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dataDir(), "*.txtar"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixture archives in %s: %v", dataDir(), err)
	}
	sort.Strings(files)
	out := map[string]map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range txtar.Parse([]byte(norm(b))).Files {
			id, part, ok := strings.Cut(file.Name, "/")
			if !ok {
				t.Fatalf("%s: fixture name %q is not id/part", f, file.Name)
			}
			if out[id] == nil {
				out[id] = map[string]string{}
			}
			if _, dup := out[id][part]; dup {
				t.Fatalf("%s: duplicate fixture part %s", f, file.Name)
			}
			out[id][part] = string(file.Data)
		}
	}
	return out
}

func validStatus(s string) bool {
	return s == "enabled" || strings.HasPrefix(s, "pending-PR")
}

// ---- runners: each returns the observed result as text, or an error ----

func runPreproc(src string) string {
	r := preproc.Process("c.gon", []byte(src))
	offs := make([]int, 0, len(r.NonNilOffsets))
	for o := range r.NonNilOffsets {
		offs = append(offs, o)
	}
	sort.Ints(offs)
	var b strings.Builder
	prev := 0
	for _, o := range offs {
		if o < prev || o > len(r.Clean) {
			return fmt.Sprintf("<offset %d out of range>", o)
		}
		b.Write(r.Clean[prev:o])
		b.WriteString(marker)
		prev = o
	}
	b.Write(r.Clean[prev:])
	return b.String()
}

func runFmt(src string) (string, error) {
	out, err := gonfmt.Format("c.gon", []byte(src))
	return string(out), err
}

// validGo requires that the clean source is valid, type-correct Go. The checker
// treats a Go type error as non-fatal and then reports no diagnostic, so a
// fixture that expects silence would otherwise hold vacuously on a source that
// does not even compile. The fixtures import nothing.
func validGo(clean []byte) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "c.go", clean, parser.AllErrors)
	if err != nil {
		return err
	}
	var first error
	conf := types.Config{Error: func(err error) {
		if first == nil {
			first = err
		}
	}}
	_, _ = conf.Check("p", fset, []*ast.File{f}, nil)
	return first
}

// runCheck returns the diagnostics as "LINE CODE" lines, sorted, or an error
// when the source cannot be checked (a syntax, type or load failure).
func runCheck(src string) (string, error) {
	r := preproc.Process("c.gon", []byte(src))
	if err := validGo(r.Clean); err != nil {
		return "", fmt.Errorf("the clean source is not valid Go: %w", err)
	}
	c, err := checker.NewWithAnnotations("c.gon", r.Clean, r.NonNilOffsets, nil)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, d := range c.Check() {
		lines = append(lines, fmt.Sprintf("%05d %s", d.Line, d.Code))
	}
	sort.Strings(lines)
	for i, l := range lines {
		n, _ := strconv.Atoi(l[:5])
		lines[i] = fmt.Sprintf("%d %s", n, l[6:])
	}
	return strings.Join(lines, "\n"), nil
}

func normalWant(s string) string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.Fields(out[i])[0])
		b, _ := strconv.Atoi(strings.Fields(out[j])[0])
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return strings.Join(out, "\n")
}

func sortedLines(s string) string { return normalWant(s) }

// holds reports whether the entry's fixture is satisfied by the current code.
func holds(e entry, fx map[string]string) (bool, string) {
	switch e.Kind {
	case "preproc":
		got := runPreproc(fx["src"])
		return got == fx["want"], got
	case "fmt":
		got, err := runFmt(fx["src"])
		if err != nil {
			return false, "error: " + err.Error()
		}
		if got != fx["want"] {
			return false, got
		}
		// A formatted source is a fixed point of the formatter.
		again, err := runFmt(got)
		if err != nil || again != got {
			return false, "not idempotent: " + again
		}
		return true, got
	case "check":
		got, err := runCheck(fx["src"])
		if err != nil {
			return false, "error: " + err.Error()
		}
		return sortedLines(got) == sortedLines(fx["want"]), got
	}
	return false, "unknown kind " + e.Kind
}

func checkable(e entry) bool { return e.Kind == "preproc" || e.Kind == "fmt" || e.Kind == "check" }

// TestEnabledFixtures runs every entry whose behaviour is already true on main.
func TestEnabledFixtures(t *testing.T) {
	fx := loadFixtures(t)
	for _, e := range loadManifest(t) {
		if e.Status != "enabled" || !checkable(e) {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			ok, got := holds(e, fx[e.ID])
			if !ok {
				t.Errorf("%s (%s, RFC %s) does not hold\n--- src\n%s--- want\n%s\n--- got\n%s",
					e.ID, e.Kind, e.RFC, fx[e.ID]["src"], fx[e.ID]["want"], got)
			}
		})
	}
}

// TestPendingFixturesAreNotYetTrue makes a pending entry meaningful: it must
// fail on the current code. When an implementation makes it pass, the same PR
// has to move it to "enabled", so a specification cannot rot as pending.
func TestPendingFixturesAreNotYetTrue(t *testing.T) {
	fx := loadFixtures(t)
	for _, e := range loadManifest(t) {
		if !strings.HasPrefix(e.Status, "pending-PR") || !checkable(e) {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			if ok, got := holds(e, fx[e.ID]); ok {
				t.Errorf("%s is %s but already holds; move it to \"enabled\" in the manifest and out of the allowlist\n--- got\n%s",
					e.ID, e.Status, got)
			}
		})
	}
}

// TestManifestIsConsistent checks the manifest against the fixtures and the
// allowlist, so that no entry can disappear or change status silently.
func TestManifestIsConsistent(t *testing.T) {
	es := loadManifest(t)
	fx := loadFixtures(t)
	seen := map[string]bool{}
	pending := map[string]string{}
	for _, e := range es {
		if e.ID == "" || seen[e.ID] {
			t.Errorf("entry id %q is empty or duplicated", e.ID)
		}
		seen[e.ID] = true
		if !validStatus(e.Status) {
			t.Errorf("%s: invalid status %q", e.ID, e.Status)
		}
		if e.RFC == "" || e.Site == "" || e.Diagnostic == "" {
			t.Errorf("%s: rfc, site and diagnostic must be filled", e.ID)
		}
		switch e.Kind {
		case "preproc", "fmt", "check":
			parts := fx[e.ID]
			if parts == nil || parts["src"] == "" {
				t.Errorf("%s: fixture has no src", e.ID)
			}
			if _, ok := parts["want"]; !ok {
				t.Errorf("%s: fixture has no want", e.ID)
			}
		case "tokenclass":
		default:
			t.Errorf("%s: unknown kind %q", e.ID, e.Kind)
		}
		if strings.HasPrefix(e.Status, "pending-PR") {
			pending[e.ID] = e.Status
		}
	}
	for id := range fx {
		if !seen[id] {
			t.Errorf("fixture %s is not in the manifest", id)
		}
	}
	b, err := os.ReadFile(filepath.Join(dataDir(), "pending-allowlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]string{}
	for _, l := range strings.Split(norm(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 2 {
			t.Errorf("allowlist line %q is not \"id status\"", l)
			continue
		}
		listed[f[0]] = f[1]
	}
	for id, st := range pending {
		if listed[id] != st {
			t.Errorf("%s is %s in the manifest but %q in the allowlist", id, st, listed[id])
		}
	}
	for id := range listed {
		if _, ok := pending[id]; !ok {
			t.Errorf("%s is in the allowlist but is not pending in the manifest", id)
		}
	}
}

// TestEveryRFCClauseHasAFixture is the traceability check: each normative
// clause of Part A that this suite covers has at least one entry.
func TestEveryRFCClauseHasAFixture(t *testing.T) {
	count := map[string]int{}
	for _, e := range loadManifest(t) {
		count[e.RFC]++
	}
	for _, c := range []string{"§6.1", "§6.2", "§6.3", "§6.4", "§6.5", "§6.6", "§6.7"} {
		if count[c] == 0 {
			t.Errorf("RFC %s has no fixture", c)
		}
	}
}

// TestConformanceSummary prints, per RFC clause, the active and pending counts
// (the conformance summary that goes in a PR description).
func TestConformanceSummary(t *testing.T) {
	type c struct{ enabled, pending int }
	m := map[string]*c{}
	var keys []string
	for _, e := range loadManifest(t) {
		if m[e.RFC] == nil {
			m[e.RFC] = &c{}
			keys = append(keys, e.RFC)
		}
		if e.Status == "enabled" {
			m[e.RFC].enabled++
		} else {
			m[e.RFC].pending++
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-5s enabled %2d  pending %2d", k, m[k].enabled, m[k].pending)
	}
}
