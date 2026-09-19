package cosy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildChatBody_MinimalInput(t *testing.T) {
	in := BuildRequestInput{
		RequestID: "req-1",
		SessionID: "sess-1",
		ModelKey:  "lite",
		Messages: []ChatMessageIn{
			{Role: "user", Content: "hi"},
		},
		CosyVersion: "1.1.34",
		BeginAt:     time.Unix(1700000000, 0),
	}
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var body chatBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RequestID != "req-1" {
		t.Errorf("request_id: got %q, want %q", body.RequestID, "req-1")
	}
	if !body.Stream {
		t.Error("stream should be true")
	}
	if body.ChatTask != "FREE_INPUT" {
		t.Errorf("chat_task: got %q, want %q", body.ChatTask, "FREE_INPUT")
	}
	if body.ModelConfig.Key != "lite" {
		t.Errorf("model key: got %q, want %q", body.ModelConfig.Key, "lite")
	}
	if body.ModelConfig.Format != "openai" {
		t.Errorf("model format: got %q, want %q", body.ModelConfig.Format, "openai")
	}
	if body.Business.Product != "cli" {
		t.Errorf("business product: got %q, want %q", body.Business.Product, "cli")
	}
	if body.Business.Version != "1.1.34" {
		t.Errorf("business version: got %q, want %q", body.Business.Version, "1.1.34")
	}
}

func TestBuildChatBody_EmptyMessages(t *testing.T) {
	_, err := BuildChatBody(BuildRequestInput{RequestID: "r"})
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
	if !errors.Is(err, ErrEmptyMessages) {
		t.Errorf("error should be ErrEmptyMessages, got: %v", err)
	}
}

func TestBuildChatBody_ZeroBeginAt(t *testing.T) {
	_, err := BuildChatBody(BuildRequestInput{
		RequestID: "r",
		Messages:  []ChatMessageIn{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for zero BeginAt")
	}
	if !errors.Is(err, ErrZeroBeginAt) {
		t.Errorf("error should be ErrZeroBeginAt, got: %v", err)
	}
}

func TestBuildChatBody_WithTools(t *testing.T) {
	tool := json.RawMessage(`{"type":"function","function":{"name":"get_weather","parameters":{}}}`)
	raw, err := BuildChatBody(BuildRequestInput{
		RequestID: "r",
		Messages:  []ChatMessageIn{{Role: "user", Content: "weather?"}},
		Tools:     []json.RawMessage{tool},
		BeginAt:   time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var body chatBody
	json.Unmarshal(raw, &body)
	if len(body.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(body.Tools))
	}
}

func TestBuildChatBody_SerializesEmptyObjects(t *testing.T) {
	raw, err := BuildChatBody(BuildRequestInput{
		RequestID: "r",
		Messages:  []ChatMessageIn{{Role: "user", Content: "hi"}},
		BeginAt:   time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["chat_context"]) != "{}" {
		t.Errorf("chat_context: got %s, want {}", m["chat_context"])
	}
	if string(m["parameters"]) != "{}" {
		t.Errorf("parameters: got %s, want {}", m["parameters"])
	}
}

func TestBuildHTTPRequest_MissingFields(t *testing.T) {
	_, err := BuildHTTPRequest(EndpointAPI2, []byte("body"), RuntimeFields{}, "req", "1.1.34")
	if err == nil {
		t.Error("expected error for incomplete runtime fields")
	}
	if !errors.Is(err, ErrIncompleteRuntimeFields) {
		t.Errorf("error should be ErrIncompleteRuntimeFields, got: %v", err)
	}
}

func TestBuildHTTPRequestAt_UnknownEndpoint(t *testing.T) {
	_, err := BuildHTTPRequestAt(Endpoint("unknown"), []byte("body"),
		RuntimeFields{EncryptUserInfo: "x", Key: "y"}, "req", "1.1.34", time.Now())
	if err == nil {
		t.Fatal("expected error for unknown endpoint")
	}
	if !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("error should be ErrUnknownEndpoint, got: %v", err)
	}
}

func TestEndpointURL(t *testing.T) {
	u2, err := endpointURL(EndpointAPI2)
	if err != nil {
		t.Fatalf("endpointURL(api2): %v", err)
	}
	u3, err := endpointURL(EndpointAPI3)
	if err != nil {
		t.Fatalf("endpointURL(api3): %v", err)
	}
	if !strings.Contains(u2, "api2.qoder.sh") {
		t.Errorf("api2 URL: %s", u2)
	}
	if !strings.Contains(u3, "api3.qoder.sh") {
		t.Errorf("api3 URL: %s", u3)
	}
	if !strings.Contains(u2, "Encode=1") {
		t.Error("URL should contain Encode=1")
	}
}

func TestEndpointURL_Unknown(t *testing.T) {
	_, err := endpointURL(Endpoint("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown endpoint")
	}
	if !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("error should be ErrUnknownEndpoint, got: %v", err)
	}
}

func TestRuntimeFieldsComplete(t *testing.T) {
	if (RuntimeFields{}).Complete() {
		t.Error("empty fields should not be complete")
	}
	if (RuntimeFields{EncryptUserInfo: "x"}).Complete() {
		t.Error("partial fields should not be complete")
	}
	if !(RuntimeFields{EncryptUserInfo: "x", Key: "y"}).Complete() {
		t.Error("both fields should be complete")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Errorf("short string: got %q", got)
	}
	if got := truncateRunes("hello world", 5); got != "hello" {
		t.Errorf("long string: got %q", got)
	}
}
