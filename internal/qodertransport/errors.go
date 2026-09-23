package qodertransport

import "time"

// StreamBusinessError is a typed error for SSE business errors that
// carries structured code/category/reset-at without exposing raw upstream body.
type StreamBusinessError struct {
	Code     int
	Category string
	ResetAt  time.Time // zero if not present
	raw      string    // private, not exposed
}

func (e *StreamBusinessError) Error() string {
	if !e.ResetAt.IsZero() {
		return "qodertransport: upstream stream error " + itoa(e.Code) + " (" + e.Category + ") reset=" + e.ResetAt.Format(time.RFC3339)
	}
	return "qodertransport: upstream stream error " + itoa(e.Code) + " (" + e.Category + ")"
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
