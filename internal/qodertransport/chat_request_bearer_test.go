package qodertransport

import (
	"testing"
)

// --- toBearerRequest tests ---

func TestToBearerRequest_AdvancedFields(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"high","max_completion_tokens":4096,"parallel_tool_calls":true`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{ID: "r1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if b.ReasoningEffort == nil || *b.ReasoningEffort != "high" {
		t.Errorf("Bearer ReasoningEffort: got %v, want %q", b.ReasoningEffort, "high")
	}
	if b.MaxCompletionTokens == nil || *b.MaxCompletionTokens != 4096 {
		t.Errorf("Bearer MaxCompletionTokens: got %v, want 4096", b.MaxCompletionTokens)
	}
	if b.ParallelToolCalls == nil || *b.ParallelToolCalls != true {
		t.Errorf("Bearer ParallelToolCalls: got %v, want true", b.ParallelToolCalls)
	}
}

func TestToBearerRequest_MaxTokensPreserved(t *testing.T) {
	raw := minimalJSON(`"max_tokens":2048,"max_completion_tokens":4096`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if b.MaxTokens == nil || *b.MaxTokens != 2048 {
		t.Errorf("Bearer MaxTokens: got %v, want 2048", b.MaxTokens)
	}
	if b.MaxCompletionTokens == nil || *b.MaxCompletionTokens != 4096 {
		t.Errorf("Bearer MaxCompletionTokens: got %v, want 4096", b.MaxCompletionTokens)
	}
}

func TestToBearerRequest_NilAdvancedFields(t *testing.T) {
	raw := minimalJSON(`"max_tokens":1024`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if b.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort should be nil, got %v", *b.ReasoningEffort)
	}
	if b.MaxCompletionTokens != nil {
		t.Errorf("MaxCompletionTokens should be nil, got %v", *b.MaxCompletionTokens)
	}
	if b.ParallelToolCalls != nil {
		t.Errorf("ParallelToolCalls should be nil, got %v", *b.ParallelToolCalls)
	}
}

func TestToBearerRequest_ToolsPassedThrough(t *testing.T) {
	raw := minimalJSON(`"tools":[{"type":"function","function":{"name":"get_weather"}}]`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if len(b.Tools) != 1 {
		t.Fatalf("Bearer Tools: got %d, want 1", len(b.Tools))
	}
	if b.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Tool name: got %q, want %q", b.Tools[0].Function.Name, "get_weather")
	}
}

func TestToBearerRequest_ToolChoicePassedThrough(t *testing.T) {
	raw := minimalJSON(`"tool_choice":"auto"`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if string(b.ToolChoice) != `"auto"` {
		t.Errorf("ToolChoice: got %s, want %q", b.ToolChoice, "auto")
	}
}

func TestToBearerRequest_MessagesPassedThrough(t *testing.T) {
	raw := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if len(b.Messages) != 1 {
		t.Fatalf("Messages: got %d, want 1", len(b.Messages))
	}
	if b.Messages[0].Role != "user" {
		t.Errorf("Message role: got %q, want %q", b.Messages[0].Role, "user")
	}
	if b.Messages[0].Content != "hello" {
		t.Errorf("Message content: got %q, want %q", b.Messages[0].Content, "hello")
	}
}

func TestToBearerRequest_RequestMetadata(t *testing.T) {
	raw := minimalJSON("")
	b, err := toBearerRequest(pMust(t, raw), StreamRequest{ID: "req-42", SessionID: "sess-99"})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if b.RequestID != "req-42" {
		t.Errorf("RequestID: got %q, want %q", b.RequestID, "req-42")
	}
	if b.SessionID != "sess-99" {
		t.Errorf("SessionID: got %q, want %q", b.SessionID, "sess-99")
	}
}

// --- Full roundtrip: parse -> toBearer serialisation check ---

func TestToBearerRequest_FullPayloadSerialisation(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"low","max_completion_tokens":8192,"parallel_tool_calls":false,"max_tokens":4096,"temperature":0.5,"tool_choice":"required"`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	b, err := toBearerRequest(p, StreamRequest{ID: "r1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("toBearerRequest: %v", err)
	}
	if b.ReasoningEffort == nil || *b.ReasoningEffort != "low" {
		t.Errorf("ReasoningEffort: %v", b.ReasoningEffort)
	}
	if b.MaxCompletionTokens == nil || *b.MaxCompletionTokens != 8192 {
		t.Errorf("MaxCompletionTokens: %v", b.MaxCompletionTokens)
	}
	if b.ParallelToolCalls == nil || *b.ParallelToolCalls != false {
		t.Errorf("ParallelToolCalls: %v", b.ParallelToolCalls)
	}
	if b.MaxTokens == nil || *b.MaxTokens != 4096 {
		t.Errorf("MaxTokens: %v", b.MaxTokens)
	}
	if b.Temperature == nil || *b.Temperature != 0.5 {
		t.Errorf("Temperature: %v", b.Temperature)
	}
	if string(b.ToolChoice) != `"required"` {
		t.Errorf("ToolChoice: %s", b.ToolChoice)
	}
}
