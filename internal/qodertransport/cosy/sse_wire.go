package cosy

import (
	"encoding/json"
	"strings"
)

// --- wire envelope (old protocol wraps payload in {"body":"..."}) ---

type wireEnvelope struct {
	Body string `json:"body"`
}

// --- OpenAI-style streaming wire format ---

type wireChoices struct {
	Choices []wireChoice `json:"choices"`
}

type wireChoice struct {
	Delta        wireDelta        `json:"delta"`
	FinishReason *json.RawMessage `json:"finish_reason,omitempty"`
}

type wireDelta struct {
	Content          *string        `json:"content"`
	ReasoningContent *string        `json:"reasoning_content"`
	ToolCalls        []wireToolCall `json:"tool_calls"`
}

type wireToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// --- usage block ---

type wireUsage struct {
	Usage wireUsageDetail `json:"usage"`
}

type wireUsageDetail struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// --- business error ---

type wireError struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
}

// safeErrorCategory maps error messages to a safe subset.
// If the message is not in the map, we return a generic category.
func safeErrorCategory(msg string) string {
	lower := strings.ToLower(strings.ReplaceAll(msg, " ", "_"))
	safe := []string{"access_denied", "rate_limited", "upstream_error", "queue_full", "timeout", "invalid_request", "context_length_exceeded"}
	for _, cat := range safe {
		if strings.Contains(lower, cat) {
			return cat
		}
	}
	return "upstream_error"
}

// unwrapBody strips the old {"body":"..."} envelope if present,
// returning the inner JSON string.
func unwrapBody(raw string) string {
	var env wireEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err == nil && env.Body != "" {
		return env.Body
	}
	return raw
}

// classifyEvent parses the unwrapped JSON payload and returns a typed SSEEvent.
func classifyEvent(payload string) SSEEvent {
	payload = unwrapBody(payload)

	// [DONE] terminal
	if isTerminal(payload) {
		return SSEEvent{Type: SSETerminal}
	}

	// Try business error first
	if ev, ok := classifyError(payload); ok {
		return ev
	}

	// Usage-only block
	if ev, ok := classifyUsage(payload); ok {
		return ev
	}

	// OpenAI-style choices
	return classifyChoices(payload)
}

// classifyError checks for {"code":...,"message":"..."} error payloads.
func classifyError(raw string) (SSEEvent, bool) {
	var we wireError
	if err := json.Unmarshal([]byte(raw), &we); err != nil || we.Code == "" {
		return SSEEvent{}, false
	}
	code, err := we.Code.Int64()
	if err != nil {
		return SSEEvent{}, false
	}
	return SSEEvent{
		Type: SSEError,
		StreamError: &StreamError{
			Code:    int(code),
			Message: safeErrorCategory(we.Message),
		},
	}, true
}

// classifyUsage checks for {"usage":{...}} payloads.
func classifyUsage(raw string) (SSEEvent, bool) {
	var wu wireUsage
	if err := json.Unmarshal([]byte(raw), &wu); err != nil {
		return SSEEvent{}, false
	}
	if wu.Usage.PromptTokens == 0 && wu.Usage.CompletionTokens == 0 && wu.Usage.TotalTokens == 0 {
		return SSEEvent{}, false
	}
	return SSEEvent{
		Type: SSEUsage,
		Usage: &Usage{
			PromptTokens:     wu.Usage.PromptTokens,
			CompletionTokens: wu.Usage.CompletionTokens,
			TotalTokens:      wu.Usage.TotalTokens,
			ReasoningTokens:  wu.Usage.CompletionTokensDetails.ReasoningTokens,
		},
	}, true
}

// classifyChoices handles {"choices":[{"delta":{...}}]}.
func classifyChoices(raw string) SSEEvent {
	var wc wireChoices
	if err := json.Unmarshal([]byte(raw), &wc); err != nil {
		return SSEEvent{Type: SSEError, StreamError: &StreamError{Message: "invalid JSON"}}
	}
	for _, ch := range wc.Choices {
		// finish_reason present → terminal
		if ch.FinishReason != nil {
			return SSEEvent{Type: SSETerminal}
		}
		d := ch.Delta
		// reasoning
		if d.ReasoningContent != nil && *d.ReasoningContent != "" {
			return SSEEvent{Type: SSEReasoningDelta, ReasoningDelta: &ReasoningDelta{Content: *d.ReasoningContent}}
		}
		// text
		if d.Content != nil && *d.Content != "" {
			return SSEEvent{Type: SSETextDelta, TextDelta: &TextDelta{Content: *d.Content}}
		}
		// tool calls
		if len(d.ToolCalls) > 0 {
			t := d.ToolCalls[0]
			return SSEEvent{
				Type: SSEToolDelta,
				ToolDelta: &ToolDelta{
					Index:     t.Index,
					ID:        t.ID,
					Name:      t.Function.Name,
					Arguments: t.Function.Arguments,
				},
			}
		}
	}
	return SSEEvent{}
}
