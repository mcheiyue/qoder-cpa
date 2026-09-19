package bearer

import "errors"

// SSEEventType enumerates event types.
type SSEEventType int

const (
	SSEUnknown SSEEventType = iota
	SSETextDelta
	SSEReasoningDelta
	SSEToolDelta
	SSEUsage
	SSETerminal
	SSEError
)

// SSEEvent is a fully typed parsed SSE event — no any/interface{}.
type SSEEvent struct {
	Type           SSEEventType
	TextDelta      *TextDelta
	ReasoningDelta *ReasoningDelta
	ToolDelta      *ToolDelta
	Usage          *Usage
	StreamError    *StreamError
	rawID          string
	rawModel       string
	rawCreated     int64
	finishReason   string
	hasFinish      bool
}

func (e *SSEEvent) rawFinishReason() (string, bool) {
	return e.finishReason, e.hasFinish
}

// TextDelta carries incremental assistant text.
type TextDelta struct {
	Content string `json:"content"`
}

// ReasoningDelta carries incremental reasoning content.
type ReasoningDelta struct {
	Content string `json:"content"`
}

// ToolDelta carries a streaming tool-call fragment.
type ToolDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// StreamError carries a typed error with a safe category.
type StreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Sentinel errors returned by the parser.
var (
	ErrEventTooLarge   = errors.New("bearer: SSE event exceeds 1 MiB limit")
	ErrMissingTerminal = errors.New("bearer: stream ended without terminal event")
)

// isTerminal reports whether the payload signals stream end.
func isTerminal(s string) bool {
	s = trimCRLF(s)
	return s == "[DONE]"
}

func trimCRLF(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

const maxEventBytes = 1 << 20 // 1 MiB
const maxLineBytes = 64 * 1024
