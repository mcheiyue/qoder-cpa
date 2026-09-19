package qodercontrol

import (
	"errors"
	"fmt"
)

// Sentinel errors for control plane operations.
var (
	ErrInvalidEndpoint   = errors.New("qodercontrol: invalid endpoint")
	ErrHostNotAllowed    = errors.New("qodercontrol: host not in allowlist")
	ErrResponseTooLarge  = errors.New("qodercontrol: response body too large")
	ErrModelUnavailable  = errors.New("qodercontrol: model catalog unavailable")
	ErrQuotaUnknown      = errors.New("qodercontrol: quota status unknown")
	ErrUpstreamFailed    = errors.New("qodercontrol: upstream request failed")
	ErrMalformedResponse = errors.New("qodercontrol: malformed response")
	ErrNonJSONResponse   = errors.New("qodercontrol: non-JSON response")
)

// UpstreamError represents a non-2xx HTTP response from Qoder.
type UpstreamError struct {
	StatusCode int
	Endpoint   string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("qodercontrol: upstream HTTP %d from %s", e.StatusCode, e.Endpoint)
}

func (e *UpstreamError) Unwrap() error { return ErrUpstreamFailed }
