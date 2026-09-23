package bearer

import (
	"encoding/json"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

type wireBearerError struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
}

func safeErrorCategory(msg string) string { return qoderstream.SafeErrorCategory(msg) }
func isRateLimitText(content string) bool { return qoderstream.IsRateLimitText(content) }

func classifyError(raw string) (SSEEvent, bool) {
	var we wireBearerError
	if err := json.Unmarshal([]byte(raw), &we); err != nil || we.Code == "" {
		return SSEEvent{}, false
	}
	var probe struct {
		Choices []struct{} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err == nil && len(probe.Choices) > 0 {
		return SSEEvent{}, false
	}
	code, err := we.Code.Int64()
	if err != nil {
		return SSEEvent{}, false
	}
	return SSEEvent{Type: SSEError, StreamError: &StreamError{
		Code: int(code), Message: safeErrorCategory(we.Message),
	}}, true
}
