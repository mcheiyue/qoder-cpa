package cosy

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

func TestSSEParser_BusinessError(t *testing.T) {
	input := "data: {\"code\":10605,\"message\":\"queue full\"}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Errorf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.Code != 10605 {
		t.Errorf("code: got %d, want 10605", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "queue_full" {
		t.Errorf("message: got %q, want %q", evt.StreamError.Message, "queue_full")
	}
}

// --- Orchids-2api statusCodeValue envelope tests ---

func TestSSEParser_StatusCodeValueBusinessError(t *testing.T) {
	// Outer envelope: {"statusCodeValue":400,"body":"{\"code\":10605,\"message\":\"queue full\"}"}
	body := `{"statusCodeValue":400,"body":"{\"code\":10605,\"message\":\"queue full\"}"}`
	input := "data: " + body + "\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.Code != 10605 {
		t.Errorf("code: got %d, want 10605", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "queue_full" {
		t.Errorf("message: got %q, want %q", evt.StreamError.Message, "queue_full")
	}
}

func TestSSEParser_StatusCodeValue429AccessDenied(t *testing.T) {
	body := `{"statusCodeValue":429,"body":"{\"code\":112,\"message\":\"access denied\"}"}`
	input := "data: " + body + "\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.Code != 112 {
		t.Errorf("code: got %d, want 112", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "access_denied" {
		t.Errorf("message: got %q, want %q", evt.StreamError.Message, "access_denied")
	}
}

func TestSSEParser_StatusCodeValueRecursiveBodyUnwrap(t *testing.T) {
	// Triple-nested: body is JSON-in-string containing another body envelope
	inner := `{"body":"{\"code\":10605,\"message\":\"queue full\"}"}`
	escaped := strings.ReplaceAll(strings.ReplaceAll(inner, `\`, `\\`), `"`, `\"`)
	outer := `{"statusCodeValue":400,"body":"` + escaped + `"}`
	input := "data: " + outer + "\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d (recursive unwrap should reach inner code/message)", evt.Type, SSEError)
	}
	if evt.StreamError == nil || evt.StreamError.Code != 10605 {
		t.Errorf("code: got %v, want 10605", evt.StreamError)
	}
}

func TestSSEParser_NestedAgentLimitResetTime(t *testing.T) {
	// message field contains a JSON-in-string with agentLimitResetTime epoch ms
	// Wire: {"statusCodeValue":429,"body":"<double-escaped-body>"}
	// body after unwrap: {"code":429,"message":"<escaped-inner>"}
	// message after JSON parse: {"msg":"rate limited","agentLimitResetTime":1727000000000}
	inner := `{"msg":"rate limited","agentLimitResetTime":1727000000000}`
	escapedInner := escapeJSON(inner)
	bodyInner := `{"code":429,"message":"` + escapedInner + `"}`
	escapedBody := escapeJSON(bodyInner)
	outer := `{"statusCodeValue":429,"body":"` + escapedBody + `"}`
	input := "data: " + outer + "\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.ResetAt.IsZero() {
		t.Error("ResetAt should be populated from agentLimitResetTime")
	}
	if evt.StreamError.ResetAt.UnixMilli() != 1727000000000 {
		t.Errorf("ResetAt: got %d, want 1727000000000", evt.StreamError.ResetAt.UnixMilli())
	}
}

func TestSSEParser_StatusOnlyAgentLimitResetTime(t *testing.T) {
	inner := `{"message":"{\"agentLimitResetTime\":1727000000000}"}`
	evt := parseBusinessTest(t, `{"statusCodeValue":401,"body":"`+escapeJSON(inner)+`"}`)
	if evt.Type != SSEError || evt.StreamError == nil || evt.StreamError.ResetAt.UnixMilli() != 1727000000000 {
		t.Fatalf("event=%+v", evt)
	}
}

func TestSSEParser_TextRateLimitPhrase(t *testing.T) {
	// Successful 200 SSE with rate-limit phrase in content field
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"available upstream accounts are rate limited\"}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d (text rate-limit phrase should emit error)", evt.Type, SSEError)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError is nil")
	}
	if evt.StreamError.Message != "rate_limited" {
		t.Errorf("message: got %q, want %q", evt.StreamError.Message, "rate_limited")
	}
}

func TestSSEParser_TextRateLimitPhraseHyphenated(t *testing.T) {
	// Variant with hyphenated "rate-limited"
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"available upstream accounts are rate-limited\"}}]}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: got %d, want %d", evt.Type, SSEError)
	}
	if evt.StreamError == nil || evt.StreamError.Message != "rate_limited" {
		t.Errorf("StreamError: got %v, want rate_limited", evt.StreamError)
	}
}

func TestSSEParser_BusinessErrorCodeString(t *testing.T) {
	input := "data: {\"code\":\"403\",\"message\":\"access denied\"}\n\n"
	parser := NewSSEParser(strings.NewReader(input))
	evt, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Fatalf("type: %d", evt.Type)
	}
	if evt.StreamError == nil {
		t.Fatal("StreamError nil")
	}
	if evt.StreamError.Code != 403 {
		t.Errorf("code: %d", evt.StreamError.Code)
	}
	if evt.StreamError.Message != "access_denied" {
		t.Errorf("message: %q", evt.StreamError.Message)
	}
}

func TestSSEParser_SignatureErrorCategory(t *testing.T) {
	parser := NewSSEParser(strings.NewReader("data: {\"code\":101,\"message\":\"signature invalid\"}\n\n"))
	event, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if event.StreamError == nil || event.StreamError.Message != "signature_invalid" {
		t.Fatalf("event=%+v, want signature_invalid", event)
	}
}
