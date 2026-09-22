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

// --- Parameters regression tests ---

func paramsInput(p Parameters) BuildRequestInput {
	return BuildRequestInput{
		RequestID: "r", Messages: []ChatMessageIn{{Role: "user", Content: "hi"}},
		BeginAt: time.Unix(1700000000, 0), Parameters: &p,
	}
}

func buildParamsRaw(t *testing.T, in BuildRequestInput) map[string]json.RawMessage {
	t.Helper()
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestParameters_MaxTokens(t *testing.T) {
	v := 4096
	m := buildParamsRaw(t, paramsInput(Parameters{MaxTokens: &v}))
	if string(m["parameters"]) != `{"max_tokens":4096}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_MaxTokensZero(t *testing.T) {
	v := 0
	m := buildParamsRaw(t, paramsInput(Parameters{MaxTokens: &v}))
	if string(m["parameters"]) != `{"max_tokens":0}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_ReasoningEffort(t *testing.T) {
	m := buildParamsRaw(t, paramsInput(Parameters{ReasoningEffort: "high"}))
	if string(m["parameters"]) != `{"reasoning_effort":"high"}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_ToolChoiceString(t *testing.T) {
	m := buildParamsRaw(t, paramsInput(Parameters{
		ToolChoice: json.RawMessage(`"auto"`),
	}))
	if string(m["parameters"]) != `{"tool_choice":"auto"}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_ToolChoiceObject(t *testing.T) {
	raw := json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`)
	m := buildParamsRaw(t, paramsInput(Parameters{ToolChoice: raw}))
	want := `{"tool_choice":{"type":"function","function":{"name":"get_weather"}}}`
	if string(m["parameters"]) != want {
		t.Errorf("parameters: got %s, want %s", m["parameters"], want)
	}
}

func TestParameters_ParallelToolCallsFalse(t *testing.T) {
	v := false
	m := buildParamsRaw(t, paramsInput(Parameters{ParallelToolCalls: &v}))
	if string(m["parameters"]) != `{"parallel_tool_calls":false}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_ParallelToolCallsTrue(t *testing.T) {
	v := true
	m := buildParamsRaw(t, paramsInput(Parameters{ParallelToolCalls: &v}))
	if string(m["parameters"]) != `{"parallel_tool_calls":true}` {
		t.Errorf("parameters: got %s", m["parameters"])
	}
}

func TestParameters_AllFields(t *testing.T) {
	maxTok := 2048
	par := false
	m := buildParamsRaw(t, paramsInput(Parameters{
		MaxTokens:         &maxTok,
		ReasoningEffort:   "low",
		ToolChoice:        json.RawMessage(`"none"`),
		ParallelToolCalls: &par,
	}))
	// Unmarshal to verify structure, exact field order doesn't matter.
	var p map[string]json.RawMessage
	if err := json.Unmarshal(m["parameters"], &p); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if string(p["max_tokens"]) != "2048" {
		t.Errorf("max_tokens: got %s", p["max_tokens"])
	}
	if string(p["reasoning_effort"]) != `"low"` {
		t.Errorf("reasoning_effort: got %s", p["reasoning_effort"])
	}
	if string(p["tool_choice"]) != `"none"` {
		t.Errorf("tool_choice: got %s", p["tool_choice"])
	}
	if string(p["parallel_tool_calls"]) != "false" {
		t.Errorf("parallel_tool_calls: got %s", p["parallel_tool_calls"])
	}
}

func TestParameters_NilPreservesEmptyObject(t *testing.T) {
	m := buildParamsRaw(t, BuildRequestInput{
		RequestID: "r", Messages: []ChatMessageIn{{Role: "user", Content: "hi"}},
		BeginAt: time.Unix(1700000000, 0),
		// Parameters deliberately nil
	})
	if string(m["parameters"]) != "{}" {
		t.Errorf("parameters with nil: got %s, want {}", m["parameters"])
	}
}

func TestParameters_EmptyStructOmitsAll(t *testing.T) {
	m := buildParamsRaw(t, paramsInput(Parameters{}))
	if string(m["parameters"]) != "{}" {
		t.Errorf("parameters with empty struct: got %s, want {}", m["parameters"])
	}
}

func TestBuildChatBody_ParametersNilBackwardCompat(t *testing.T) {
	// Verify the full body JSON matches existing format when Parameters is nil.
	raw, err := BuildChatBody(BuildRequestInput{
		RequestID: "req-1", SessionID: "sess-1", ModelKey: "lite",
		Messages:    []ChatMessageIn{{Role: "user", Content: "hi"}},
		CosyVersion: "1.1.34", BeginAt: time.Unix(1700000000, 0),
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var body chatBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(body.Parameters) != "{}" {
		t.Errorf("parameters backward compat: got %s, want {}", body.Parameters)
	}
	if body.RequestID != "req-1" {
		t.Errorf("request_id: got %q", body.RequestID)
	}
}
