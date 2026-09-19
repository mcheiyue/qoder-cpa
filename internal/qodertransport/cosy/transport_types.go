package cosy

import (
	"fmt"
	"strings"
	"time"
)

// Clock returns the current time. Transport injects this for test determinism.
type Clock func() time.Time

// SecurityCategory classifies HTTP errors without exposing raw server text.
type SecurityCategory string

const (
	CatAuthFailure   SecurityCategory = "auth_failure"
	CatRateLimited   SecurityCategory = "rate_limited"
	CatUpstreamError SecurityCategory = "upstream_error"
	CatForbidden     SecurityCategory = "forbidden"
	CatUnknown       SecurityCategory = "unknown"
)

// HTTPError represents a non-2xx HTTP response.
// Body is kept internal for classification only; never exposed to callers.
type HTTPError struct {
	StatusCode int
	category   SecurityCategory
	readErr    error
}

func (e *HTTPError) Error() string {
	return "cosy: HTTP " + itoa(e.StatusCode)
}

// Category returns the safe error classification.
func (e *HTTPError) Category() SecurityCategory {
	return e.category
}

func (e *HTTPError) Unwrap() error {
	return e.readErr
}

// InvalidContentTypeError indicates that the upstream did not return SSE.
// The media type is retained for diagnostics; the response body is never retained.
type InvalidContentTypeError struct {
	ContentType string
}

func (e *InvalidContentTypeError) Error() string {
	if e.ContentType == "" {
		return "cosy: missing text/event-stream content type"
	}
	return fmt.Sprintf("cosy: unexpected content type %q", e.ContentType)
}

// itoa is a minimal int-to-string to avoid importing strconv in this file.
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

// classifyHTTPStatus maps a status code to a SecurityCategory.
func classifyHTTPStatus(code int) SecurityCategory {
	switch {
	case code == 401:
		return CatAuthFailure
	case code == 403:
		return CatForbidden
	case code == 429:
		return CatRateLimited
	case code >= 500:
		return CatUpstreamError
	default:
		return CatUnknown
	}
}

// isLoopbackURL reports whether rawURL is http(s) over a loopback address
// with no userinfo, no query, and no fragment.
func isLoopbackURL(rawURL string) bool {
	if len(rawURL) < 8 {
		return false
	}
	scheme := rawURL[:7]
	if scheme == "http://" {
		// ok
	} else if len(rawURL) >= 8 {
		scheme = rawURL[:8]
		if scheme == "https://" {
			// ok
		} else {
			return false
		}
	} else {
		return false
	}
	rest := rawURL[len(scheme):]

	// reject userinfo
	if strings.ContainsRune(rest, '@') {
		return false
	}

	// reject query and fragment anywhere (before host/path split)
	if strings.ContainsAny(rest, "?#") {
		return false
	}

	// split host/path
	slashIdx := strings.IndexByte(rest, '/')
	var host string
	if slashIdx < 0 {
		host = rest
	} else {
		host = rest[:slashIdx]
	}

	// strip port
	if colonIdx := strings.IndexByte(host, ':'); colonIdx >= 0 {
		host = host[:colonIdx]
	}

	return isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	if host == "[::1]" {
		return true
	}
	if strings.HasPrefix(host, "[::ffff:127") {
		return true
	}
	return false
}
