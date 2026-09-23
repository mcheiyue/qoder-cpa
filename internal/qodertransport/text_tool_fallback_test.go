package qodertransport

import (
	"io"
	"strconv"
	"strings"
	"testing"
)

// seqHandle yields chunks in order, then EOF.
type seqHandle struct {
	chunks [][]byte
	idx    int
}

func (h *seqHandle) ReadChunk() ([]byte, error) {
	if h.idx >= len(h.chunks) {
		return nil, io.EOF
	}
	c := h.chunks[h.idx]
	h.idx++
	return c, nil
}

func (h *seqHandle) Cancel() {}

// errHandle returns chunks then a persistent error.
type errHandle struct {
	chunks [][]byte
	idx    int
	err    error
}

func (h *errHandle) ReadChunk() ([]byte, error) {
	if h.idx < len(h.chunks) {
		c := h.chunks[h.idx]
		h.idx++
		return c, nil
	}
	return nil, h.err
}

func (h *errHandle) Cancel() {}

func sseC(json string) []byte { return []byte("data: " + json + "\n\n") }

func textD(id, model, content string) []byte {
	return sseC(`{"id":"` + id + `","object":"chat.completion.chunk","model":"` + model +
		`","choices":[{"index":0,"delta":{"content":` + strconv.Quote(content) + `}}]}`)
}

func finishD(id, model, reason string) []byte {
	return sseC(`{"id":"` + id + `","object":"chat.completion.chunk","model":"` + model +
		`","choices":[{"index":0,"delta":{},"finish_reason":"` + reason + `"}]}`)
}

func toolD(id, model string, idx int, callID, name, args string) []byte {
	return sseC(`{"id":"` + id + `","object":"chat.completion.chunk","model":"` + model +
		`","choices":[{"index":0,"delta":{"tool_calls":[{"index":` + strconv.Itoa(idx) +
		`,"id":"` + callID + `","type":"function","function":{"name":"` + name +
		`","arguments":` + strconv.Quote(args) + `}}]}}]}`)
}

func collect(h StreamHandle) ([][]byte, error) {
	var out [][]byte
	for {
		c, err := h.ReadChunk()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, c)
	}
}

func hasSub(chunks [][]byte, sub string) bool {
	for _, c := range chunks {
		if strings.Contains(string(c), sub) {
			return true
		}
	}
	return false
}

func TestTextToolFallback_SplitPrefixBuffering(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool"),
		textD("c1", "m", " calls: "),
		textD("c1", "m", `[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]`),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"tool_calls"`) || !hasSub(chunks, `"name":"search"`) {
		t.Fatalf("expected tool_calls with search, got: %q", chunks)
	}
	if !hasSub(chunks, `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected tool_calls finish, got: %q", chunks)
	}
}

func TestTextToolFallback_FencedJSON(t *testing.T) {
	fence := "```"
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool calls: "+fence+"json\n"),
		textD("c1", "m", `[{"id":"call_1","type":"function","function":{"name":"exec","arguments":"{}"}}]`),
		textD("c1", "m", "\n"+fence),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"tool_calls"`) || !hasSub(chunks, `"name":"exec"`) {
		t.Fatalf("expected tool_calls with exec, got: %q", chunks)
	}
}

func TestTextToolFallback_ToolsDisabled(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool calls: something"),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, false, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"content":"Tool calls: something"`) || !hasSub(chunks, `"finish_reason":"stop"`) {
		t.Fatalf("text should pass through untouched, got: %q", chunks)
	}
}

func TestTextToolFallback_Divergence(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool calls are"),
		textD("c1", "m", " important"),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if hasSub(chunks, `"tool_calls"`) {
		t.Fatalf("divergent text must not emit tool_calls, got: %q", chunks)
	}
	if !hasSub(chunks, `"content":`) {
		t.Fatalf("expected text content, got: %q", chunks)
	}
	if !hasSub(chunks, `"finish_reason":"stop"`) {
		t.Fatalf("expected stop finish, got: %q", chunks)
	}
}

func TestTextToolFallback_NativeToolSuppression(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool calls: "),
		textD("c1", "m", `[{"id":"call_buf"`),
		toolD("c1", "m", 0, "call_native", "native_tool", "{}"),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"name":"native_tool"`) {
		t.Fatalf("expected native tool, got: %q", chunks)
	}
	for _, c := range chunks {
		if strings.Contains(string(c), `"name":"call_buf"`) {
			t.Fatalf("buffered fallback must be suppressed, got: %q", chunks)
		}
	}
}

func TestTextToolFallback_MultipleCalls(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", `Tool calls: [{"id":"c1","type":"function","function":{"name":"a","arguments":"{}"}},{"id":"c2","type":"function","function":{"name":"b","arguments":"{}"}}]`),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSub(chunks, `"name":"a"`) || !hasSub(chunks, `"name":"b"`) {
		t.Fatalf("expected both tool calls a and b, got: %q", chunks)
	}
}

func TestTextToolFallback_MalformedPayload(t *testing.T) {
	inner := &seqHandle{chunks: [][]byte{
		textD("c1", "m", "Tool calls: not valid json!!"),
		finishD("c1", "m", "stop"),
		doneSSE(),
	}}
	h := newTextToolFallback(inner, true, "c1", "m")
	chunks, err := collect(h)
	if err != nil {
		t.Fatal(err)
	}
	if hasSub(chunks, `"tool_calls"`) {
		t.Fatalf("malformed payload must not emit tool_calls, got: %q", chunks)
	}
	if !hasSub(chunks, `"content":`) {
		t.Fatalf("expected text content, got: %q", chunks)
	}
}
