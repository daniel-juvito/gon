package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/daniel-juvito/gon/internal/checker"
	"github.com/daniel-juvito/gon/internal/gna"
	"github.com/daniel-juvito/gon/internal/preproc"
)

type Server struct {
	in   *bufio.Reader
	out  io.Writer
	mu   sync.Mutex
	docs map[string]string
}

func New(in io.Reader, out io.Writer) *Server {
	return &Server{in: bufio.NewReader(in), out: out, docs: make(map[string]string)}
}

func (s *Server) Run() error {
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := s.handle(msg); err != nil {
			return err
		}
	}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (s *Server) readMessage() (*rpcMessage, error) {
	var contentLength int
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(s.in, buf); err != nil {
		return nil, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *Server) writeMessage(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func (s *Server) handle(msg *rpcMessage) error {
	switch msg.Method {
	case "initialize":
		return s.writeMessage(map[string]any{
			"jsonrpc": "2.0",
			"id":      jsonRaw(msg.ID),
			"result": map[string]any{
				"capabilities": map[string]any{"textDocumentSync": 1},
				"serverInfo":   map[string]string{"name": "gon", "version": "1.3.0-dev"},
			},
		})
	case "initialized":
		return nil
	case "shutdown":
		return s.writeMessage(map[string]any{"jsonrpc": "2.0", "id": jsonRaw(msg.ID), "result": nil})
	case "exit":
		os.Exit(0)
		return nil
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		s.docs[p.TextDocument.URI] = p.TextDocument.Text
		return s.publishDiagnostics(p.TextDocument.URI, p.TextDocument.Text)
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			return err
		}
		if len(p.ContentChanges) == 0 {
			return nil
		}
		text := p.ContentChanges[len(p.ContentChanges)-1].Text
		s.docs[p.TextDocument.URI] = text
		return s.publishDiagnostics(p.TextDocument.URI, text)
	default:
		return nil
	}
}

func (s *Server) publishDiagnostics(uri, text string) error {
	return s.writeMessage(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params":  map[string]any{"uri": uri, "diagnostics": Analyze(uri, text)},
	})
}

type Diagnostic struct {
	Range struct {
		Start Position `json:"start"`
		End   Position `json:"end"`
	} `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func Analyze(uri, text string) []Diagnostic {
	filename := uri
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		filename = uri[i+1:]
	}
	if !strings.HasSuffix(filename, ".gon") {
		filename += ".gon"
	}
	result := preproc.Process(filename, []byte(text))
	c, err := checker.NewWithAnnotations(filename, result.Clean, result.NonNilOffsets, gna.DefaultChain(".", "."))
	if err != nil {
		return []Diagnostic{{Severity: 1, Source: "gon", Message: err.Error()}}
	}
	raw := c.Check()
	out := make([]Diagnostic, 0, len(raw))
	for _, d := range raw {
		ld := Diagnostic{Severity: 1, Code: d.Code, Source: "gon", Message: d.Message}
		if d.Severity == checker.SeverityWarning {
			ld.Severity = 2
		}
		line, col := d.Line-1, d.Col-1
		if line < 0 {
			line = 0
		}
		if col < 0 {
			col = 0
		}
		ld.Range.Start = Position{Line: line, Character: col}
		ld.Range.End = Position{Line: line, Character: col + 1}
		out = append(out, ld)
	}
	return out
}

func jsonRaw(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal(id, &v)
	return v
}
