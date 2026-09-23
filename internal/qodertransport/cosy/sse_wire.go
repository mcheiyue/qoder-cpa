package cosy

import (
	"encoding/json"
	"time"
)

type wireEnvelope struct {
	Body string `json:"body"`
}

type wireStatusEnvelope struct {
	StatusCodeValue int    `json:"statusCodeValue"`
	Body            string `json:"body"`
}

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

type wireError struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
}

// unwrapBody recursively strips {"body":"..."} envelopes up to depth 3.
func unwrapBody(raw string) string {
	current := raw
	for range 3 {
		var env wireEnvelope
		if err := json.Unmarshal([]byte(current), &env); err == nil && env.Body != "" {
			current = env.Body
		} else {
			break
		}
	}
	return current
}

func extractStatusCodeValue(raw string) int {
	var env wireStatusEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err == nil && env.StatusCodeValue != 0 {
		return env.StatusCodeValue
	}
	return 0
}

// extractAgentLimitResetTime searches for agentLimitResetTime (epoch millis)
// in a JSON string that may itself be JSON-in-string, up to depth 3.
func extractAgentLimitResetTime(raw string) time.Time {
	current := raw
	for range 3 {
		var wrapper struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(current), &wrapper); err != nil || wrapper.Message == "" {
			break
		}
		if t := parseResetTime(wrapper.Message); !t.IsZero() {
			return t
		}
		current = wrapper.Message
	}
	if t := parseResetTime(raw); !t.IsZero() {
		return t
	}
	return time.Time{}
}

func classifyEvent(payload string) SSEEvent {
	statusCode := extractStatusCodeValue(payload)
	payload = unwrapBody(payload)

	if isTerminal(payload) {
		return SSEEvent{Type: SSETerminal}
	}

	if ev, ok := classifyError(payload); ok {
		ev.StreamError.ResetAt = extractAgentLimitResetTime(payload)
		return ev
	}

	if statusCode != 0 && statusCode != 200 {
		resetAt := extractAgentLimitResetTime(payload)
		return SSEEvent{
			Type:        SSEError,
			StreamError: &StreamError{Code: statusCode, Message: statusCategory(statusCode), ResetAt: resetAt},
		}
	}

	if ev, ok := classifyUsage(payload); ok {
		return ev
	}

	return classifyChoices(payload)
}

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

func classifyChoices(raw string) SSEEvent {
	var wc wireChoices
	if err := json.Unmarshal([]byte(raw), &wc); err != nil {
		return SSEEvent{Type: SSEError, StreamError: &StreamError{Message: "invalid JSON"}}
	}
	for _, ch := range wc.Choices {
		if ch.FinishReason != nil {
			return SSEEvent{Type: SSETerminal}
		}
		d := ch.Delta
		if d.ReasoningContent != nil && *d.ReasoningContent != "" {
			return SSEEvent{Type: SSEReasoningDelta, ReasoningDelta: &ReasoningDelta{Content: *d.ReasoningContent}}
		}
		if d.Content != nil && *d.Content != "" {
			if isRateLimitText(*d.Content) {
				return SSEEvent{
					Type:        SSEError,
					StreamError: &StreamError{Code: 429, Message: "rate_limited"},
				}
			}
			return SSEEvent{Type: SSETextDelta, TextDelta: &TextDelta{Content: *d.Content}}
		}
		if len(d.ToolCalls) > 0 {
			t := d.ToolCalls[0]
			return SSEEvent{
				Type: SSEToolDelta,
				ToolDelta: &ToolDelta{
					Index: t.Index, ID: t.ID, Name: t.Function.Name, Arguments: t.Function.Arguments,
				},
			}
		}
	}
	return SSEEvent{}
}
