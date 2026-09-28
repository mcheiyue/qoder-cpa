package qodertransport

import (
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

// StreamBusinessError is a typed error for SSE business errors that
// carries structured code/category/reset-at without exposing raw upstream body.
type StreamBusinessError struct {
	Code      int
	Category  string
	ResetAt   time.Time                 // zero if not present
	OuterCode int                       // outer HTTP/wrapper status (0 when absent)
	Queue     *qoderstream.QueuePayload // structured queue payload when present
	raw       string                    // private, not exposed
}

func (e *StreamBusinessError) Error() string {
	base := "qodertransport: upstream stream error " + itoa(e.Code) + " (" + e.Category + ")"
	if e.Queue != nil {
		if e.Queue.RetryAfterSeconds > 0 {
			base += " retry=" + itoa(e.Queue.RetryAfterSeconds) + "s"
		} else if e.Queue.WaitTime > 0 {
			base += " wait=" + itoa(e.Queue.WaitTime) + "s"
		}
	}
	if !e.ResetAt.IsZero() {
		base += " reset=" + e.ResetAt.Format(time.RFC3339)
	}
	return base
}

// itoa is a minimal int-to-string to avoid importing strconv here.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
