package qoderstream

import (
	"net/http"
	"testing"
	"time"
)

func TestClassifyBusiness_Queue10605WithPayload(t *testing.T) {
	raw := `{"code":10605,"message":"service busy","isQueued":true,"queueType":"p3","serviceAvailable":false,"retryAfterSeconds":30,"waitTime":30}`
	cat, q := ClassifyBusiness(10605, "service busy", raw)
	if cat != CatQueueUnavailable {
		t.Errorf("category: got %q, want %q", cat, CatQueueUnavailable)
	}
	if q == nil {
		t.Fatal("queue payload is nil")
	}
	if !q.IsQueued || q.QueueType != "p3" || q.ServiceAvailable || q.RetryAfterSeconds != 30 || q.WaitTime != 30 {
		t.Errorf("queue payload: %+v", q)
	}
}

func TestClassifyBusiness_QueuePayloadNestedInMessage(t *testing.T) {
	// Queue fields arrive inside a JSON-in-string message wrapper.
	raw := `{"code":429,"message":"{\"isQueued\":true,\"queueType\":\"p1\",\"serviceAvailable\":false,\"retryAfterSeconds\":15,\"waitTime\":15}"}`
	cat, q := ClassifyBusiness(429, "queued", raw)
	if cat != CatQueueUnavailable {
		t.Errorf("category: got %q, want %q", cat, CatQueueUnavailable)
	}
	if q == nil || q.QueueType != "p1" || q.RetryAfterSeconds != 15 {
		t.Errorf("queue payload: %+v", q)
	}
}

func TestClassifyBusiness_112WithPricingURL(t *testing.T) {
	raw := `{"code":112,"message":"plan required","pricingUrl":"https://example.com/buy"}`
	cat, _ := ClassifyBusiness(112, "plan required", raw)
	if cat != CatModelUnavailable {
		t.Errorf("category: got %q, want %q", cat, CatModelUnavailable)
	}
}

func TestClassifyBusiness_112AccessDenied(t *testing.T) {
	cat, _ := ClassifyBusiness(112, "access denied", `{"code":112,"message":"access denied"}`)
	if cat != "access_denied" {
		t.Errorf("category: got %q, want access_denied", cat)
	}
}

func TestClassifyBusiness_Direct429(t *testing.T) {
	cat, q := ClassifyBusiness(429, "slow down", `{"code":429,"message":"slow down"}`)
	if cat != "rate_limited" {
		t.Errorf("category: got %q, want rate_limited", cat)
	}
	if q != nil {
		t.Errorf("queue payload should be nil, got %+v", q)
	}
}

func TestClassifyBusiness_DailyCountExceeded(t *testing.T) {
	cat, _ := ClassifyBusiness(10010, "Billing daily count exceeded for this account.", `{}`)
	if cat != CatDailyLimit {
		t.Errorf("category: got %q, want %q", cat, CatDailyLimit)
	}
}

func TestClassifyBusiness_NoUsablePlan(t *testing.T) {
	cat, _ := ClassifyBusiness(10011, "no usable plan or allowance", `{}`)
	if cat != CatPlanRequired {
		t.Errorf("category: got %q, want %q", cat, CatPlanRequired)
	}
}

func TestClassifyBusiness_StatusFallback(t *testing.T) {
	if cat, _ := ClassifyBusiness(403, "", `{"msg":"denied"}`); cat != "access_denied" {
		t.Errorf("403: got %q, want access_denied", cat)
	}
	if cat, _ := ClassifyBusiness(400, "", `{}`); cat != "invalid_request" {
		t.Errorf("400: got %q, want invalid_request", cat)
	}
	if cat, _ := ClassifyBusiness(500, "boom", `{}`); cat != "upstream_error" {
		t.Errorf("500: got %q, want upstream_error", cat)
	}
}

func TestParseRetryAfter_DeltaSeconds(t *testing.T) {
	if got := ParseRetryAfter("42", time.Now()); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	hdr := now.Add(90 * time.Second).Format(http.TimeFormat)
	got := ParseRetryAfter(hdr, now)
	if got < 88 || got > 90 {
		t.Errorf("got %d, want ~90", got)
	}
}

func TestParseRetryAfter_PastHTTPDate(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	hdr := now.Add(-90 * time.Second).Format(http.TimeFormat)
	if got := ParseRetryAfter(hdr, now); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	for _, hdr := range []string{"", "abc", "-5"} {
		if got := ParseRetryAfter(hdr, time.Now()); got != 0 {
			t.Errorf("%q: got %d, want 0", hdr, got)
		}
	}
}
