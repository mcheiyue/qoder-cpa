package qoderstream

import "strings"

// IsRateLimitText checks if content contains the Orchids-2api rate-limit phrase.
func IsRateLimitText(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "available upstream accounts are rate-limited") ||
		strings.Contains(lower, "available upstream accounts are rate limited")
}

// SafeErrorCategory maps error messages to a safe subset.
func SafeErrorCategory(msg string) string {
	lower := strings.ToLower(strings.ReplaceAll(msg, " ", "_"))
	safe := []string{"access_denied", "rate_limited", "upstream_error", "queue_full", "timeout", "invalid_request", "context_length_exceeded", "signature_invalid"}
	for _, cat := range safe {
		if strings.Contains(lower, cat) {
			return cat
		}
	}
	return "upstream_error"
}

// StatusCategory maps a non-200 statusCodeValue to a safe error category.
func StatusCategory(code int) string {
	switch {
	case code == 401 || code == 403:
		return "access_denied"
	case code == 429:
		return "rate_limited"
	case code >= 500:
		return "upstream_error"
	default:
		return "upstream_error"
	}
}
