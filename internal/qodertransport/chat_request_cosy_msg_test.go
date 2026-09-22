package qodertransport

import (
	"testing"
)

// --- toCosyRequest system message extraction ---

func TestToCosyRequest_SystemMessageExtraction(t *testing.T) {
	raw := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":"Be helpful"},{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.SystemPrompt != "Be helpful" {
		t.Errorf("SystemPrompt: got %q, want %q", c.SystemPrompt, "Be helpful")
	}
	if len(c.Messages) != 1 {
		t.Fatalf("Messages: got %d, want 1", len(c.Messages))
	}
	if c.Messages[0].Role != "user" {
		t.Errorf("Message role: got %q, want %q", c.Messages[0].Role, "user")
	}
}

func TestToCosyRequest_MultipleSystemMessages(t *testing.T) {
	raw := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":"A"},{"role":"system","content":"B"},{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.SystemPrompt != "A\nB" {
		t.Errorf("SystemPrompt: got %q, want %q", c.SystemPrompt, "A\nB")
	}
}

func TestToCosyRequest_DeveloperMessageAsSystem(t *testing.T) {
	raw := []byte(`{"model":"gpt-4","messages":[{"role":"developer","content":"You are a pirate"},{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.SystemPrompt != "You are a pirate" {
		t.Errorf("SystemPrompt: got %q, want %q", c.SystemPrompt, "You are a pirate")
	}
}
