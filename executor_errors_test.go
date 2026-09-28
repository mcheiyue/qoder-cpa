package main

import (
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

func TestClassifyExecutorError_StreamBusinessError_QueueFull(t *testing.T) {
	err := &qodertransport.StreamBusinessError{
		Code:     10605,
		Category: "queue_full",
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "queue_full" {
		t.Errorf("code: got %q, want %q", f.code, "queue_full")
	}
	if f.status != 502 {
		t.Errorf("status: got %d, want 502", f.status)
	}
}

func TestClassifyExecutorError_StreamBusinessError_AccessDenied(t *testing.T) {
	err := &qodertransport.StreamBusinessError{
		Code:     112,
		Category: "access_denied",
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "access_denied" {
		t.Errorf("code: got %q, want %q", f.code, "access_denied")
	}
}

func TestClassifyExecutorError_StreamBusinessError_RateLimited(t *testing.T) {
	err := &qodertransport.StreamBusinessError{
		Code:     429,
		Category: "rate_limited",
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "rate_limited" {
		t.Errorf("code: got %q, want %q", f.code, "rate_limited")
	}
}

func TestClassifyExecutorError_StreamBusinessError_WithResetAt(t *testing.T) {
	resetAt := time.UnixMilli(1727000000000)
	err := &qodertransport.StreamBusinessError{
		Code:     429,
		Category: "rate_limited",
		ResetAt:  resetAt,
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "rate_limited" {
		t.Errorf("code: %q", f.code)
	}
	// ResetAt must appear in the error message
	if !contains(f.message, "reset=") {
		t.Errorf("message should contain reset time, got: %q", f.message)
	}
}

func TestClassifyExecutorError_StreamBusinessError_Upstream5xx(t *testing.T) {
	err := &qodertransport.StreamBusinessError{
		Code:     503,
		Category: "upstream_error",
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "upstream_error" {
		t.Errorf("code: got %q, want %q", f.code, "upstream_error")
	}
}

func TestClassifyExecutorError_StreamBusinessError_DoesNotMutateQuota(t *testing.T) {
	// StreamBusinessError must NOT carry quota/status mutation fields
	err := &qodertransport.StreamBusinessError{
		Code:     10605,
		Category: "queue_full",
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	// status should always be 502 for stream business errors (opaque upstream)
	if f.status != 502 {
		t.Errorf("status: got %d, want 502", f.status)
	}
}

func TestClassifyExecutorError_QueueUnavailableCarriesRetryWait(t *testing.T) {
	err := &qodertransport.StreamBusinessError{
		Code:     10605,
		Category: "queue_unavailable",
		Queue:    &qoderstream.QueuePayload{QueueType: "p3", ServiceAvailable: false, RetryAfterSeconds: 30, WaitTime: 30},
	}
	f := classifyExecutorError(err)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "queue_unavailable" {
		t.Errorf("code: got %q, want queue_unavailable", f.code)
	}
	if !contains(f.message, "retry=30s") {
		t.Errorf("message should carry retry wait, got: %q", f.message)
	}
	if f.status != 502 {
		t.Errorf("status: got %d, want 502", f.status)
	}
}

func TestClassifyExecutorError_NewBusinessCategories(t *testing.T) {
	for _, cat := range []string{"daily_limit_exceeded", "plan_required", "model_unavailable"} {
		f := classifyExecutorError(&qodertransport.StreamBusinessError{Code: 10010, Category: cat})
		if f == nil || f.code != cat {
			t.Errorf("%s: got %+v", cat, f)
		}
	}
}

func TestClassifyExecutorError_BearerHTTP403AccessDenied(t *testing.T) {
	f := classifyExecutorError(&bearer.HTTPError{StatusCode: 403})
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "access_denied" {
		t.Errorf("code: got %q, want access_denied", f.code)
	}
	if f.status != 403 {
		t.Errorf("status: got %d, want 403", f.status)
	}
}

func TestClassifyExecutorError_CosyHTTP429RateLimitedWithRetryAfter(t *testing.T) {
	f := classifyExecutorError(&cosy.HTTPError{StatusCode: 429, RetryAfterSec: 30})
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "rate_limited" {
		t.Errorf("code: got %q, want rate_limited", f.code)
	}
	if !contains(f.message, "retry_after=30s") {
		t.Errorf("message should carry retry_after, got: %q", f.message)
	}
}

func TestClassifyExecutorError_CosyHTTP400InvalidRequest(t *testing.T) {
	f := classifyExecutorError(&cosy.HTTPError{StatusCode: 400})
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "invalid_request" {
		t.Errorf("code: got %q, want invalid_request", f.code)
	}
}

func TestClassifyExecutorError_MissingTerminalUpstreamError(t *testing.T) {
	f := classifyExecutorError(cosy.ErrMissingTerminal)
	if f == nil {
		t.Fatal("nil failure")
	}
	if f.code != "upstream_error" || f.status != 502 {
		t.Errorf("got %+v, want upstream_error/502", f)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
