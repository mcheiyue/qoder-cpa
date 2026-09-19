package cosy

import (
	"io"
	"strings"
	"testing"
)

func TestSSEParser_TextDelta(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSETextDelta)
	}
	if evt.TextDelta == nil || evt.TextDelta.Content != "hello" {
		t.Errorf("content: got %q, want %q", evt.TextDelta, "hello")
	}
}

func TestSSEParser_ReasoningDelta(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEReasoningDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSEReasoningDelta)
	}
	if evt.ReasoningDelta == nil || evt.ReasoningDelta.Content != "thinking" {
		t.Errorf("content: got %q, want %q", evt.ReasoningDelta, "thinking")
	}
}

func TestSSEParser_ToolDelta(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"city\\\":\\\"NYC\\\"}\"}}]}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEToolDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSEToolDelta)
	}
	td := evt.ToolDelta
	if td == nil {
		t.Fatal("ToolDelta is nil")
	}
	if td.Index != 0 {
		t.Errorf("index: %d", td.Index)
	}
	if td.ID != "call_1" {
		t.Errorf("id: %q", td.ID)
	}
	if td.Name != "get_weather" {
		t.Errorf("name: %q", td.Name)
	}
	if !strings.Contains(td.Arguments, "NYC") {
		t.Errorf("arguments: %q", td.Arguments)
	}
}

func TestSSEParser_ToolNameOnly(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"do_thing\"}}]}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEToolDelta {
		t.Errorf("type: got %d, want SSEToolDelta", evt.Type)
	}
	if evt.ToolDelta == nil || evt.ToolDelta.Name != "do_thing" {
		t.Errorf("name: %v", evt.ToolDelta)
	}
}

func TestSSEParser_ToolArgsOnly(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"x\\\":1}\"}}]}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEToolDelta {
		t.Errorf("type: got %d, want SSEToolDelta", evt.Type)
	}
	if evt.ToolDelta == nil || !strings.Contains(evt.ToolDelta.Arguments, "x") {
		t.Errorf("args: %v", evt.ToolDelta)
	}
}

func TestSSEParser_Usage(t *testing.T) {
	input := "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20,\"total_tokens\":30,\"completion_tokens_details\":{\"reasoning_tokens\":5}}}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEUsage {
		t.Errorf("type: got %d, want %d", evt.Type, SSEUsage)
	}
	if evt.Usage == nil {
		t.Fatal("Usage is nil")
	}
	if evt.Usage.PromptTokens != 10 || evt.Usage.CompletionTokens != 20 || evt.Usage.ReasoningTokens != 5 {
		t.Errorf("usage: %+v", evt.Usage)
	}
}

func TestSSEParser_Terminal(t *testing.T) {
	input := "data: [DONE]\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETerminal {
		t.Errorf("type: got %d, want %d", evt.Type, SSETerminal)
	}
	_, err = parser.Parse()
	if err != io.EOF {
		t.Errorf("after terminal: got err %v, want EOF", err)
	}
}

func TestSSEParser_BodyWrapper(t *testing.T) {
	inner := "{\"choices\":[{\"delta\":{\"content\":\"wrapped\"}}]}"
	input := "data: {\"body\":\"" + escapeJSON(inner) + "\"}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: got %d, want %d", evt.Type, SSETextDelta)
	}
}

func TestSSEParser_BusinessError(t *testing.T) {
	input := "data: {\"code\":10605,\"message\":\"queue full\"}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Errorf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.Code != 10605 {
		t.Errorf("code: got %d, want 10605", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "queue_full" {
		t.Errorf("message: got %q, want %q", evt.StreamError.Message, "queue_full")
	}
}

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

func TestSSEParser_InvalidJSON(t *testing.T) {
	input := "data: not json at all\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Errorf("type: got %d, want SSEError", evt.Type)
	}
}
