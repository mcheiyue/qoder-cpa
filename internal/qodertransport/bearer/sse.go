package bearer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// SSEParser is an incremental SSE parser with context support and size limits.
type SSEParser struct {
	reader       *bufio.Reader
	data         bytes.Buffer
	event        string
	seenTerminal bool
	aborted      bool
}

// NewSSEParser wraps a reader in an incremental SSE parser.
func NewSSEParser(r io.Reader) *SSEParser {
	return &SSEParser{
		reader: bufio.NewReaderSize(r, 64*1024),
	}
}

// Parse reads the next event using context.Background().
func (p *SSEParser) Parse() (SSEEvent, error) {
	return p.ParseContext(context.Background())
}

// ParseContext reads the next SSE event, respecting ctx cancellation.
func (p *SSEParser) ParseContext(ctx context.Context) (SSEEvent, error) {
	if p.aborted {
		return SSEEvent{}, io.EOF
	}
	for {
		if err := ctx.Err(); err != nil {
			return SSEEvent{}, err
		}
		line, err := p.reader.ReadString('\n')
		if len(line) > maxLineBytes+2 {
			p.data.Reset()
			p.aborted = true
			return SSEEvent{}, io.ErrShortBuffer
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if p.data.Len() > 0 {
				return p.dispatch(), nil
			}
			if err == io.EOF {
				return p.eofFallback()
			}
			continue
		}
		if strings.HasPrefix(trimmed, "event:") {
			p.event = strings.TrimSpace(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "data:") {
			d := strings.TrimSpace(trimmed[5:])
			if p.data.Len() > 0 {
				p.data.WriteByte('\n')
			}
			if _, err := p.data.WriteString(d); err != nil {
				return SSEEvent{}, err
			}
			if p.data.Len() > maxEventBytes {
				p.aborted = true
				return SSEEvent{}, ErrEventTooLarge
			}
		}
		if err == io.EOF {
			if p.data.Len() > 0 {
				return p.dispatch(), nil
			}
			return p.eofFallback()
		}
	}
}

func (p *SSEParser) eofFallback() (SSEEvent, error) {
	if p.seenTerminal {
		p.aborted = true
		return SSEEvent{}, io.EOF
	}
	p.aborted = true
	return SSEEvent{}, ErrMissingTerminal
}

func (p *SSEParser) dispatch() SSEEvent {
	raw := strings.TrimSpace(p.data.String())
	p.data.Reset()
	p.event = ""

	if isTerminal(raw) {
		p.seenTerminal = true
		return SSEEvent{Type: SSETerminal}
	}

	ev := classifyEvent(raw)
	if ev.Type == SSETerminal {
		p.seenTerminal = true
	}
	return ev
}

// classifyEvent parses the JSON payload and returns a typed SSEEvent.
func classifyEvent(payload string) SSEEvent {
	if isTerminal(payload) {
		return SSEEvent{Type: SSETerminal}
	}

	if ev, ok := classifyError(payload); ok {
		return ev
	}

	// Usage-only block (no choices, just usage)
	if ev, ok := classifyUsage(payload); ok {
		return ev
	}

	// OpenAI-style choices with optional finish_reason
	return classifyChoices(payload)
}

func classifyUsage(raw string) (SSEEvent, bool) {
	var wu wireUsageBlock
	if err := json.Unmarshal([]byte(raw), &wu); err != nil {
		return SSEEvent{}, false
	}
	if len(wu.Choices) > 0 {
		return SSEEvent{}, false
	}
	if wu.Usage == nil || (wu.Usage.PromptTokens == 0 && wu.Usage.CompletionTokens == 0 && wu.Usage.TotalTokens == 0) {
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
		rawID:      wu.ID,
		rawModel:   wu.Model,
		rawCreated: wu.Created,
	}, true
}

func classifyChoices(raw string) SSEEvent {
	var wc wireChunk
	if err := json.Unmarshal([]byte(raw), &wc); err != nil {
		return SSEEvent{Type: SSEError, StreamError: &StreamError{Message: "invalid JSON"}}
	}
	ev := SSEEvent{
		rawID:      wc.ID,
		rawModel:   wc.Model,
		rawCreated: wc.Created,
	}
	for _, ch := range wc.Choices {
		if ch.FinishReason != nil {
			ev.Type = SSETerminal
			ev.finishReason = *ch.FinishReason
			ev.hasFinish = true
			return ev
		}
		d := ch.Delta
		if d.ReasoningContent != nil && *d.ReasoningContent != "" {
			ev.Type = SSEReasoningDelta
			ev.ReasoningDelta = &ReasoningDelta{Content: *d.ReasoningContent}
			return ev
		}
		if d.Content != nil && *d.Content != "" {
			// Check for rate-limit phrase before emitting
			if isRateLimitText(*d.Content) {
				ev.Type = SSEError
				ev.StreamError = &StreamError{
					Code:    429,
					Message: "rate_limited",
				}
				return ev
			}
			ev.Type = SSETextDelta
			ev.TextDelta = &TextDelta{Content: *d.Content}
			return ev
		}
		if len(d.ToolCalls) > 0 {
			t := d.ToolCalls[0]
			ev.Type = SSEToolDelta
			ev.ToolDelta = &ToolDelta{
				Index:     t.Index,
				ID:        t.ID,
				Name:      t.Function.Name,
				Arguments: t.Function.Arguments,
			}
			return ev
		}
	}
	return ev
}

// wireUsageBlock is used to detect usage-only chunks (no choices).
type wireUsageBlock struct {
	ID      string     `json:"id"`
	Model   string     `json:"model"`
	Created int64      `json:"created"`
	Choices []struct{} `json:"choices"`
	Usage   *wireUsage `json:"usage,omitempty"`
}
