package cosy

import (
	"strings"
	"testing"
)

func TestSSEParser_Queue10605StructuredEnvelope(t *testing.T) {
	inner := `{"code":10605,"message":"queue is full","isQueued":true,"queueType":"p3","serviceAvailable":false,"retryAfterSeconds":30,"waitTime":30}`
	body := `{"statusCodeValue":403,"body":"` + escapeJSON(inner) + `"}`
	evt := parseBusinessTest(t, body)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	se := evt.StreamError
	if se.Code != 10605 {
		t.Errorf("code: got %d, want 10605", se.Code)
	}
	if se.Message != "queue_unavailable" {
		t.Errorf("message: got %q, want queue_unavailable", se.Message)
	}
	if se.OuterCode != 403 {
		t.Errorf("outer code: got %d, want 403", se.OuterCode)
	}
	if se.Queue == nil {
		t.Fatal("queue payload is nil")
	}
	if se.Queue.QueueType != "p3" || se.Queue.RetryAfterSeconds != 30 || se.Queue.WaitTime != 30 || se.Queue.ServiceAvailable || !se.Queue.IsQueued {
		t.Errorf("queue payload: %+v", se.Queue)
	}
}

func TestSSEParser_StatusOnlyQueuePayload(t *testing.T) {
	// Outer 403 with queue payload but no inner code must still be queue_unavailable.
	inner := `{"message":"busy","queueType":"p3","serviceAvailable":false,"retryAfterSeconds":30}`
	body := `{"statusCodeValue":403,"body":"` + escapeJSON(inner) + `"}`
	evt := parseBusinessTest(t, body)
	if evt.Type != SSEError || evt.StreamError == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.StreamError.Message != "queue_unavailable" {
		t.Errorf("message: got %q, want queue_unavailable", evt.StreamError.Message)
	}
	if evt.StreamError.OuterCode != 403 {
		t.Errorf("outer code: got %d, want 403", evt.StreamError.OuterCode)
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
		t.Fatalf("first frame type: got %d, want SSETextDelta (phrase incomplete)", first.Type)
	}
	second, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse 2: %v", err)
	}
	if second.Type != SSEError || second.StreamError == nil {
		t.Fatalf("second frame: got %+v, want SSEError rate_limited", second)
	}
	if second.StreamError.Code != 429 || second.StreamError.Message != "rate_limited" {
		t.Errorf("second frame error: got %+v, want 429 rate_limited", second.StreamError)
	}
}

func TestSSEParser_UsageCachedTokens(t *testing.T) {
	payload := `{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}}}`
	evt := parseBusinessTest(t, payload)
	if evt.Type != SSEUsage || evt.Usage == nil {
		t.Fatalf("event=%+v", evt)
	}
	if evt.Usage.PromptTokens != 10 || evt.Usage.CompletionTokens != 5 || evt.Usage.TotalTokens != 15 {
		t.Errorf("usage: %+v", evt.Usage)
	}
	if evt.Usage.CachedTokens != 4 {
		t.Errorf("cached_tokens: got %d, want 4", evt.Usage.CachedTokens)
	}
	if evt.Usage.ReasoningTokens != 2 {
		t.Errorf("reasoning_tokens: got %d, want 2", evt.Usage.ReasoningTokens)
	}
}
