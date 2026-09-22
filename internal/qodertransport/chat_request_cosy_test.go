package qodertransport

import (
	"encoding/json"
	"testing"
)

// --- toCosyRequest tests ---

func TestToCosyRequest_AdvancedParameters(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"high","parallel_tool_calls":true`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{ID: "r1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil when fields are set")
	}
	if c.Parameters.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort: got %q, want %q", c.Parameters.ReasoningEffort, "high")
	}
	if c.Parameters.ParallelToolCalls == nil || *c.Parameters.ParallelToolCalls != true {
		t.Errorf("ParallelToolCalls: got %v, want true", c.Parameters.ParallelToolCalls)
	}
}

func TestToCosyRequest_MaxCompletionTokensPriority(t *testing.T) {
	raw := minimalJSON(`"max_tokens":1024,"max_completion_tokens":4096`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	if c.Parameters.MaxTokens == nil || *c.Parameters.MaxTokens != 4096 {
		t.Errorf("COSY MaxTokens: got %v, want 4096 (max_completion_tokens should win)", c.Parameters.MaxTokens)
	}
}

func TestToCosyRequest_MaxTokensOnly(t *testing.T) {
	raw := minimalJSON(`"max_tokens":2048`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	if c.Parameters.MaxTokens == nil || *c.Parameters.MaxTokens != 2048 {
		t.Errorf("COSY MaxTokens: got %v, want 2048", c.Parameters.MaxTokens)
	}
}

func TestToCosyRequest_ToolChoicePassedThrough(t *testing.T) {
	raw := minimalJSON(`"tool_choice":"auto"`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	if string(c.Parameters.ToolChoice) != `"auto"` {
		t.Errorf("ToolChoice: got %s, want %q", c.Parameters.ToolChoice, "auto")
	}
}

func TestToCosyRequest_ToolChoiceObjectPassedThrough(t *testing.T) {
	raw := minimalJSON(`"tool_choice":{"type":"function","function":{"name":"get_weather"}}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	var tc struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(c.Parameters.ToolChoice, &tc); err != nil {
		t.Fatalf("unmarshal tool_choice: %v", err)
	}
	if tc.Type != "function" || tc.Function.Name != "get_weather" {
		t.Errorf("ToolChoice: got %+v, want function/get_weather", tc)
	}
}

func TestToCosyRequest_ParallelToolCallsExplicitFalse(t *testing.T) {
	raw := minimalJSON(`"parallel_tool_calls":false`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	if c.Parameters.ParallelToolCalls == nil {
		t.Fatal("ParallelToolCalls should not be nil")
	}
	if *c.Parameters.ParallelToolCalls != false {
		t.Errorf("ParallelToolCalls: got true, want false")
	}
}

func TestToCosyRequest_NoParamsWhenAllNil(t *testing.T) {
	raw := minimalJSON("")
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters != nil {
		t.Errorf("COSY Parameters should be nil when no fields set, got %+v", c.Parameters)
	}
}

func TestToCosyRequest_TemperatureNotSent(t *testing.T) {
	raw := minimalJSON(`"temperature":0.9`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.Temperature == nil || *p.Temperature != 0.9 {
		t.Fatalf("parseChatPayload should have captured temperature")
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters != nil {
		t.Errorf("COSY Parameters should be nil (temperature not a COSY param), got %+v", c.Parameters)
	}
}

func TestToCosyRequest_ToolsPassedThrough(t *testing.T) {
	raw := minimalJSON(`"tools":[{"type":"function","function":{"name":"search"}}]`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if len(c.Tools) != 1 {
		t.Fatalf("Tools: got %d, want 1", len(c.Tools))
	}
	var tool map[string]json.RawMessage
	if err := json.Unmarshal(c.Tools[0], &tool); err != nil {
		t.Fatalf("unmarshal tool: %v", err)
	}
}

func TestToCosyRequest_ModelConfig(t *testing.T) {
	raw := minimalJSON("")
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.ModelConfig.Key != "gpt-4" {
		t.Errorf("ModelConfig.Key: got %q, want %q", c.ModelConfig.Key, "gpt-4")
	}
	if c.ModelConfig.Format != "openai" {
		t.Errorf("ModelConfig.Format: got %q, want %q", c.ModelConfig.Format, "openai")
	}
	if c.ModelConfig.Source != "system" {
		t.Errorf("ModelConfig.Source: got %q, want %q", c.ModelConfig.Source, "system")
	}
}

func TestToCosyRequest_RequestMetadata(t *testing.T) {
	raw := minimalJSON("")
	c, err := toCosyRequest(pMust(t, raw), StreamRequest{ID: "r1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.RequestID != "r1" {
		t.Errorf("RequestID: got %q, want %q", c.RequestID, "r1")
	}
	if c.SessionID != "s1" {
		t.Errorf("SessionID: got %q, want %q", c.SessionID, "s1")
	}
}

// --- Full roundtrip: parse -> toCosy serialisation check ---

func TestToCosyRequest_FullPayloadParameters(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"medium","max_completion_tokens":2048,"parallel_tool_calls":true,"max_tokens":1024,"tool_choice":"none"`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatalf("toCosyRequest: %v", err)
	}
	if c.Parameters == nil {
		t.Fatal("COSY Parameters should not be nil")
	}
	if c.Parameters.ReasoningEffort != "medium" {
		t.Errorf("ReasoningEffort: got %q, want %q", c.Parameters.ReasoningEffort, "medium")
	}
	if c.Parameters.MaxTokens == nil || *c.Parameters.MaxTokens != 2048 {
		t.Errorf("MaxTokens: got %v, want 2048 (max_completion_tokens wins)", c.Parameters.MaxTokens)
	}
	if c.Parameters.ParallelToolCalls == nil || *c.Parameters.ParallelToolCalls != true {
		t.Errorf("ParallelToolCalls: %v", c.Parameters.ParallelToolCalls)
	}
	if string(c.Parameters.ToolChoice) != `"none"` {
		t.Errorf("ToolChoice: %s", c.Parameters.ToolChoice)
	}
}
