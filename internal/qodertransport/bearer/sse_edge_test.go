package bearer

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestSSEParser_CrossChunk(t *testing.T) {
	r := &chunkReader{
		chunks: []string{
			"data: {\"choices\":[{\"delta\":{\"content\":",
			"\"chunked text\"}}]}\n",
			"\n",
		},
	}
	parser := NewSSEParser(r)
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSETextDelta)
	}
	if evt.TextDelta == nil || evt.TextDelta.Content != "chunked text" {
		t.Errorf("content: got %q, want %q", evt.TextDelta, "chunked text")
	}
}

func TestSSEParser_CRLF(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"crlf\"}}]}\r\n\r\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSETextDelta)
	}
}

func TestSSEParser_NoTerminal(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: %d", evt.Type)
	}
	_, err = parser.Parse()
	if err != ErrMissingTerminal {
		t.Errorf("expected ErrMissingTerminal, got %v", err)
	}
}

func TestSSEParser_MultipleEvents(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\ndata: [DONE]\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt1, _ := parser.Parse()
	if evt1.Type != SSETextDelta {
		t.Errorf("event 1 type: %d", evt1.Type)
	}
	evt2, _ := parser.Parse()
	if evt2.Type != SSETextDelta {
		t.Errorf("event 2 type: %d", evt2.Type)
	}
	evt3, _ := parser.Parse()
	if evt3.Type != SSETerminal {
		t.Errorf("event 3 type: %d, want terminal", evt3.Type)
	}
}

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

func TestSSEParser_ToolNameAndArgsTogether(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"test\\\"}\"}}]}}]}\n\n"
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
