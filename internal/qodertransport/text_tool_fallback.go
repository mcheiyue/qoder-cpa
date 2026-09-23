package qodertransport

import (
	"encoding/json"
	"io"
	"strings"
)

const (
	maxToolFallbackBuf   = 4 << 20 // 4 MiB
	maxToolFallbackCalls = 128
)

type fbState int

const (
	fbIdle fbState = iota
	fbBuffering
	fbEmitted
)

type toolCallInfo struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// textToolFallbackHandle wraps a StreamHandle to detect when the upstream emits
// tool calls as plain text (Tool calls: [...]) instead of native tool_calls events.
// When tools are enabled and the prefix is detected, text deltas are buffered until
// a complete JSON array can be parsed, then emitted as standard tool_calls chunks.
type textToolFallbackHandle struct {
	inner    StreamHandle
	id       string
	model    string
	state    fbState
	buf      []byte
	pending  [][]byte
	savedErr error
	done     bool
}

// newTextToolFallback wraps inner with text-based tool-call fallback.
// If toolsEnabled is false, returns inner unchanged (zero overhead).
func newTextToolFallback(inner StreamHandle, toolsEnabled bool, id, model string) StreamHandle {
	if !toolsEnabled {
		return inner
	}
	return &textToolFallbackHandle{inner: inner, id: id, model: model}
}

func (h *textToolFallbackHandle) ReadChunk() ([]byte, error) {
	if len(h.pending) > 0 {
		c := h.pending[0]
		h.pending = h.pending[1:]
		return c, nil
	}
	if h.savedErr != nil {
		err := h.savedErr
		h.savedErr = nil
		return nil, err
	}
	if h.done {
		return nil, io.EOF
	}

	raw, err := h.inner.ReadChunk()
	if err != nil {
		if h.state == fbBuffering && len(h.buf) > 0 {
			h.pending = buildTextChunks(h.id, h.model, string(h.buf))
			h.buf = h.buf[:0]
			h.state = fbIdle
			h.savedErr = err
			if len(h.pending) > 0 {
				c := h.pending[0]
				h.pending = h.pending[1:]
				return c, nil
			}
		}
		return nil, err
	}

	payload, isDone := parseFallbackSSE(raw)
	if isDone {
		if h.state == fbBuffering && len(h.buf) > 0 {
			h.pending = buildTextChunks(h.id, h.model, string(h.buf))
			h.buf = h.buf[:0]
			h.state = fbIdle
			h.pending = append(h.pending, raw)
			h.done = true
			if len(h.pending) > 0 {
				c := h.pending[0]
				h.pending = h.pending[1:]
				return c, nil
			}
		}
		h.done = true
		return raw, nil
	}

	var cc chatChunk
	if json.Unmarshal(payload, &cc) != nil || len(cc.Choices) == 0 {
		return raw, nil // usage or unparseable — pass through
	}
	choice := cc.Choices[0]

	// Native tool_calls: suppress any pending text buffer
	if len(choice.Delta.ToolCalls) > 0 {
		if h.state == fbBuffering {
			h.buf = h.buf[:0]
		}
		h.state = fbEmitted
		return raw, nil
	}

	// Finish chunk
	if choice.FinishReason != nil {
		if h.state == fbBuffering && len(h.buf) > 0 {
			h.pending = buildTextChunks(h.id, h.model, string(h.buf))
			h.buf = h.buf[:0]
			h.state = fbIdle
			h.pending = append(h.pending, raw)
			c := h.pending[0]
			h.pending = h.pending[1:]
			return c, nil
		}
		if h.state == fbEmitted {
			return rewriteFinish(raw, "tool_calls"), nil
		}
		return raw, nil
	}

	// Text delta
	if choice.Delta.Content == "" {
		return raw, nil
	}

	switch h.state {
	case fbEmitted:
		return h.ReadChunk() // discard text after tool calls

	case fbBuffering:
		h.buf = append(h.buf, choice.Delta.Content...)
		if len(h.buf) > maxToolFallbackBuf {
			h.pending = buildTextChunks(h.id, h.model, string(h.buf))
			h.buf = h.buf[:0]
			h.state = fbIdle
			c := h.pending[0]
			h.pending = h.pending[1:]
			return c, nil
		}
		if calls, ok := tryParseToolCalls(h.buf); ok {
			h.pending = buildToolCallChunks(h.id, h.model, calls)
			h.buf = h.buf[:0]
			h.state = fbEmitted
			c := h.pending[0]
			h.pending = h.pending[1:]
			return c, nil
		}
		if !bufferMatchesPrefix(h.buf) {
			h.pending = buildTextChunks(h.id, h.model, string(h.buf))
			h.buf = h.buf[:0]
			h.state = fbIdle
			c := h.pending[0]
			h.pending = h.pending[1:]
			return c, nil
		}
		return h.ReadChunk()

	default: // fbIdle
		h.buf = append(h.buf, choice.Delta.Content...)
		if calls, ok := tryParseToolCalls(h.buf); ok {
			h.pending = buildToolCallChunks(h.id, h.model, calls)
			h.buf = h.buf[:0]
			h.state = fbEmitted
			if len(h.pending) > 0 {
				c := h.pending[0]
				h.pending = h.pending[1:]
				return c, nil
			}
		}
		if bufferMatchesPrefix(h.buf) {
			h.state = fbBuffering
			return h.ReadChunk()
		}
		if isToolCallsPrefix(h.buf) {
			return h.ReadChunk() // still accumulating prefix chars
		}
		chunks := buildTextChunks(h.id, h.model, string(h.buf))
		h.buf = h.buf[:0]
		if len(chunks) > 0 {
			return chunks[0], nil
		}
		return h.ReadChunk()
	}
}

func (h *textToolFallbackHandle) Cancel() { h.inner.Cancel() }

// --- helpers ---

func parseFallbackSSE(raw []byte) ([]byte, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "data: [DONE]" || s == "[DONE]" {
		return nil, true
	}
	if strings.HasPrefix(s, "data:") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "data:"))
	}
	return []byte(s), false
}

// isToolCallsPrefix reports whether buf is a prefix of "Tool calls:" (exact).
func isToolCallsPrefix(buf []byte) bool {
	const prefix = "Tool calls:"
	if len(buf) > len(prefix) {
		return false
	}
	return strings.HasPrefix(prefix, string(buf))
}
