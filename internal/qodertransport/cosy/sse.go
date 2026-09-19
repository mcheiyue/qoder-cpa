package cosy

import (
	"bufio"
	"bytes"
	"context"
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
		// Check line length before trimming.
		if len(line) > maxLineBytes+2 { // +2 for \r\n
			p.data.Reset()
			p.aborted = true
			return SSEEvent{}, io.ErrShortBuffer
		}
		trimmed := strings.TrimRight(line, "\r\n")

		if trimmed == "" {
			// Blank line: dispatch accumulated data.
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
		// timer: line is ignored.

		if err == io.EOF {
			if p.data.Len() > 0 {
				return p.dispatch(), nil
			}
			return p.eofFallback()
		}
	}
}

// eofFallback returns the right error when the stream ends.
func (p *SSEParser) eofFallback() (SSEEvent, error) {
	if p.seenTerminal {
		p.aborted = true
		return SSEEvent{}, io.EOF
	}
	p.aborted = true
	return SSEEvent{}, ErrMissingTerminal
}

// dispatch builds a typed SSEEvent from the accumulated data buffer.
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
