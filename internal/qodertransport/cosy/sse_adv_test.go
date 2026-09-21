package cosy

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestSSEParser_ContextCancel(t *testing.T) {
	r := &blockingReader{block: make(chan struct{})}
	parser := NewSSEParser(r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := parser.ParseContext(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSSEParser_OversizeEvent(t *testing.T) {
	lines := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		lines = append(lines, "data: "+strings.Repeat("B", 30*1024))
	}
	input := strings.Join(lines, "\n") + "\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	_, err := parser.Parse()
	if err != ErrEventTooLarge {
		t.Errorf("expected ErrEventTooLarge, got %v", err)
	}
}

func TestSSEParser_TerminalThenEOF(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt1, _ := parser.Parse()
	if evt1.Type != SSETextDelta {
		t.Errorf("event 1: %d", evt1.Type)
	}
	evt2, _ := parser.Parse()
	if evt2.Type != SSETerminal {
		t.Errorf("event 2: %d, want terminal", evt2.Type)
	}
	_, err := parser.Parse()
	if err != io.EOF {
		t.Errorf("after terminal: %v, want EOF", err)
	}
}

func TestSSEParser_FinishReasonTerminal(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETerminal {
		t.Errorf("type: got %d, want SSETerminal", evt.Type)
	}
	_, err = parser.Parse()
	if err != io.EOF {
		t.Errorf("after terminal: %v, want EOF", err)
	}
}

func TestSSEParser_ToolNameAndArgsTogether(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"test\\\"}\"}}]}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEToolDelta {
		t.Fatalf("type: %d", evt.Type)
	}
	td := evt.ToolDelta
	if td.Name != "search" || td.Arguments == "" || td.ID != "c1" {
		t.Errorf("tool: name=%q id=%q args=%q", td.Name, td.ID, td.Arguments)
	}
}

func TestSSEParser_BusinessErrorCodeString(t *testing.T) {
	input := "data: {\"code\":\"403\",\"message\":\"access denied\"}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: %d", evt.Type)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError nil")
	}
	if evt.StreamError.Code != 403 {
		t.Errorf("code: %d", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "access_denied" {
		t.Errorf("message: %q", evt.StreamError.Message)
	}
}

func TestSSEParser_SignatureErrorCategory(t *testing.T) {
	parser := NewSSEParser(strings.NewReader("data: {\"code\":101,\"message\":\"signature invalid\"}\n\n"))
	event, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if event.StreamError == nil || event.StreamError.Message != "signature_invalid" {
		t.Fatalf("event=%+v, want signature_invalid", event)
	}
}

// --- helpers ---

type chunkReader struct {
	chunks []string
	idx    int
	off    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.idx]
	n := copy(p, chunk[r.off:])
	r.off += n
	if r.off >= len(chunk) {
		r.idx++
		r.off = 0
	}
	return n, nil
}

type blockingReader struct {
	block chan struct{}
}

func (r *blockingReader) Read(p []byte) (int, error) {
	<-r.block
	return 0, io.EOF
}

func escapeJSON(s string) string {
	b, _ := io.ReadAll(strings.NewReader(s))
	_ = b
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}
