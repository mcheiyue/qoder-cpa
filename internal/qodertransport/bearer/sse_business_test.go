package bearer

import (
	"strings"
	"testing"
)

func parseBusinessTest(t *testing.T, payload string) SSEEvent {
	t.Helper()
	evt, err := NewSSEParser(strings.NewReader("data: " + payload + "\n\n")).Parse()
	if err != nil {
		t.Fatal(err)
	}
	return evt
}

func TestSSEParser_BusinessError10605(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":10605,"message":"queue_full"}`)
	if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.Code != 10605 || evt.StreamError.Message != "queue_full" {
		t.Fatalf("event=%+v", evt)
	}
}

func TestSSEParser_BusinessErrorAccessDenied(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":112,"message":"access denied"}`)
	if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.Code != 112 || evt.StreamError.Message != "access_denied" {
		t.Fatalf("event=%+v", evt)
	}
}

func TestSSEParser_BusinessErrorStringCode(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":"429","message":"rate limited"}`)
	if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.Code != 429 || evt.StreamError.Message != "rate_limited" {
		t.Fatalf("event=%+v", evt)
	}
}

func TestSSEParser_TextRateLimitPhrases(t *testing.T) {
	for _, text := range []string{"available upstream accounts are rate limited", "available upstream accounts are rate-limited"} {
		evt := parseBusinessTest(t, `{"choices":[{"delta":{"content":"`+text+`"}}]}`)
		if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.Code != 429 || evt.StreamError.Message != "rate_limited" {
			t.Fatalf("text=%q event=%+v", text, evt)
		}
	}
}

func TestSSEParser_BusinessError5xx(t *testing.T) {
	evt := parseBusinessTest(t, `{"code":503,"message":"service unavailable"}`)
	if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.Code != 503 || evt.StreamError.Message != "upstream_error" {
		t.Fatalf("event=%+v", evt)
	}
}
