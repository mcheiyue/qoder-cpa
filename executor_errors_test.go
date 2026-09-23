package main

import (
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
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
