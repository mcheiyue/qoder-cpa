package cosy

import (
	"errors"
	"time"
)

// SSEEventKind enumerates event types.
type SSEEventType int

const (
	SSEUnknown SSEEventType = iota
	SSETextDelta
	SSEReasoningDelta
	SSEToolCall
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
// A single chunk may carry any combination of index/id/name/arguments.
type ToolDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// Usage carries token counts.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
}

// StreamError carries a typed error with a safe category.
type StreamError struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	ResetAt time.Time `json:"-"` // populated from agentLimitResetTime when present
}

// Sentinel errors returned by the parser.
var (
	ErrEventTooLarge   = errors.New("cosy: SSE event exceeds 1 MiB limit")
	ErrMissingTerminal = errors.New("cosy: stream ended without terminal event")
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

// maxEventBytes is the per-event data accumulator limit.
const maxEventBytes = 1 << 20 // 1 MiB

// maxLineBytes is the max single-line length before we reject.
const maxLineBytes = 64 * 1024
