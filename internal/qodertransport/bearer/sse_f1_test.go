package bearer

import (
	"strings"
	"testing"
)

func TestSSEParser_112PricingURLModelUnavailable(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":112,"message":"plan required","pricingUrl":"https://example.com/buy"}`)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.StreamError.Code != 112 {
		t.Errorf("code: got %d, want 112", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "model_unavailable" {
		t.Errorf("message: got %q, want model_unavailable", evt.StreamError.Message)
	}
}

func TestSSEParser_DailyCountExceeded(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":10010,"message":"Billing daily count exceeded for this account."}`)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.StreamError.Message != "daily_limit_exceeded" {
		t.Errorf("message: got %q, want daily_limit_exceeded", evt.StreamError.Message)
	}
}

func TestSSEParser_NoUsablePlan(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":10011,"message":"no usable plan or allowance"}`)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.StreamError.Message != "plan_required" {
		t.Errorf("message: got %q, want plan_required", evt.StreamError.Message)
	}
}

func TestSSEParser_Queue10605Structured(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":10605,"message":"queue is full","isQueued":true,"queueType":"p3","serviceAvailable":false,"retryAfterSeconds":30,"waitTime":30}`)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	se := evt.StreamError
	if se.Code != 10605 || se.Message != "queue_unavailable" {
		t.Errorf("error: got %+v, want 10605 queue_unavailable", se)
	}
	if se.Queue == nil || se.Queue.QueueType != "p3" || se.Queue.RetryAfterSeconds != 30 || se.Queue.WaitTime != 30 || se.Queue.ServiceAvailable || !se.Queue.IsQueued {
		t.Errorf("queue payload: %+v", se.Queue)
	}
}

func TestSSEParser_CrossChunkRateLimitPhrase(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"available upstream accounts are rate-\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"limited, try again later\"}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	first, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse 1: %v", err)
	}
	if first.Type != SSETextDelta {
		t.Fatalf("first frame type: got %d, want SSETextDelta", first.Type)
	}
	second, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse 2: %v", err)
	}
	if second.Type != SSEError || second.StreamError == nil {
		t.Fatalf("second frame: got %+v, want SSEError", second)
	}
	if second.StreamError.Code != 429 || second.StreamError.Message != "rate_limited" {
		t.Errorf("error: got %+v, want 429 rate_limited", second.StreamError)
	}
}

func TestSSEParser_UsageCachedTokens(t *testing.T) {
	payload := `{"id":"c1","model":"m","created":1,"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}}}`
	evt := parseBusinessTest(t, payload)
	if evt.Type != SSEUsage || evt.Usage == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.Usage.CachedTokens != 4 || evt.Usage.ReasoningTokens != 2 {
		t.Errorf("usage: %+v", evt.Usage)
	}
	if evt.Usage.PromptTokens != 10 || evt.Usage.CompletionTokens != 5 || evt.Usage.TotalTokens != 15 {
		t.Errorf("usage tokens: %+v", evt.Usage)
	}
}
