package lsp

import (
	"bytes"
	"strings"
	"testing"
)

func TestAnalyzeReportsGN002(t *testing.T) {
	diags := Analyze("file:///tmp/t.gon", `package main
type S struct { Client !*int }
var _ = S{}
`)
	found := false
	for _, d := range diags {
		if d.Code == "GN002" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected GN002, got %#v", diags)
	}
}

func TestAnalyzeCleanFile(t *testing.T) {
	diags := Analyze("file:///tmp/t.gon", `package main
func f(x *int) {}
`)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", diags)
	}
}

func TestServerInitializeDidOpenShutdown(t *testing.T) {
	var in, out bytes.Buffer
	frames := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///t.gon","text":"package main\ntype S struct { X !*int }\nvar _ = S{}\n"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"shutdown","params":null}`,
	}
	for _, f := range frames {
		in.WriteString("Content-Length: ")
		in.WriteString(itoa(len(f)))
		in.WriteString("\r\n\r\n")
		in.WriteString(f)
	}
	_ = New(&in, &out).Run()
	resp := out.String()
	if !strings.Contains(resp, "publishDiagnostics") {
		t.Fatalf("expected publishDiagnostics:\n%s", resp)
	}
	if !strings.Contains(resp, "GN002") {
		t.Fatalf("expected GN002:\n%s", resp)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
