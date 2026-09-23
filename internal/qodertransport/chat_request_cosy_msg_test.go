package qodertransport

import (
	"encoding/json"
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

// --- COSY model metadata propagation ---

func TestToCosyRequest_ReasoningModelSetsModelConfigAndParameters(t *testing.T) {
	raw := []byte(`{"model":"qoder/r1","messages":[{"role":"user","content":"hi"}]}`)
	resolver := ModelResolver(func(publicID string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, MaxInputTokens: 131072}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !c.ModelConfig.IsReasoning {
		t.Error("ModelConfig.IsReasoning = false, want true")
	}
	if c.ModelConfig.MaxInputTokens != 131072 {
		t.Errorf("ModelConfig.MaxInputTokens = %d, want 131072", c.ModelConfig.MaxInputTokens)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters is nil, want non-nil")
	}
	if c.Parameters.EnableThinking == nil || !*c.Parameters.EnableThinking {
		t.Error("EnableThinking = nil/false, want true")
	}
	if c.Parameters.ContextLength == nil || *c.Parameters.ContextLength != 131072 {
		t.Errorf("ContextLength = %v, want 131072", c.Parameters.ContextLength)
	}
}

func TestToCosyRequest_NonReasoningModelNoThinkingOrContext(t *testing.T) {
	raw := []byte(`{"model":"qoder/p1","messages":[{"role":"user","content":"hi"}]}`)
	resolver := ModelResolver(func(publicID string) ResolvedModel {
		return ResolvedModel{InternalID: "p1", IsReasoning: false, MaxInputTokens: 0}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.ModelConfig.IsReasoning {
		t.Error("ModelConfig.IsReasoning = true, want false")
	}
	if c.ModelConfig.MaxInputTokens != 0 {
		t.Errorf("ModelConfig.MaxInputTokens = %d, want 0", c.ModelConfig.MaxInputTokens)
	}
	// Parameters should be nil when no metadata fields and no other params set
	if c.Parameters != nil {
		if c.Parameters.EnableThinking != nil {
			t.Errorf("EnableThinking = %v, want nil", c.Parameters.EnableThinking)
		}
		if c.Parameters.ContextLength != nil {
			t.Errorf("ContextLength = %v, want nil", c.Parameters.ContextLength)
		}
	}
}

func TestToCosyRequest_ReasoningEffortNoneDisablesThinking(t *testing.T) {
	raw := []byte(`{"model":"qoder/r1","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`)
	resolver := ModelResolver(func(publicID string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, MaxInputTokens: 64000}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters is nil")
	}
	if c.Parameters.EnableThinking == nil || *c.Parameters.EnableThinking {
		t.Errorf("EnableThinking = %v, want false", c.Parameters.EnableThinking)
	}
	// reasoning_effort "none" should NOT appear in the JSON output
	rawParams, _ := json.Marshal(c.Parameters)
	if string(rawParams) != "{}" {
		// Should only have enable_thinking, no reasoning_effort
		var m map[string]any
		json.Unmarshal(rawParams, &m)
		if _, ok := m["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent for 'none', got %s", rawParams)
		}
	}
}

func TestToCosyRequest_ReasoningEffortNoneDisablesThinkingForPlainModel(t *testing.T) {
	raw := []byte(`{"model":"qoder/p1","messages":[{"role":"user","content":"hi"}],"reasoning_effort":" NONE "}`)
	resolver := ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "p1"}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil || c.Parameters.EnableThinking == nil || *c.Parameters.EnableThinking {
		t.Fatalf("EnableThinking=%v, want false", c.Parameters)
	}
}

func TestToCosyRequest_ReasoningEffortMediumForwardsBoth(t *testing.T) {
	raw := []byte(`{"model":"qoder/r1","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"medium"}`)
	resolver := ModelResolver(func(publicID string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, MaxInputTokens: 64000}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters is nil")
	}
	if c.Parameters.EnableThinking == nil || !*c.Parameters.EnableThinking {
		t.Errorf("EnableThinking = %v, want true", c.Parameters.EnableThinking)
	}
	if c.Parameters.ReasoningEffort != "medium" {
		t.Errorf("ReasoningEffort = %q, want 'medium'", c.Parameters.ReasoningEffort)
	}
}

func TestToCosyRequest_ContextLengthOmittedWhenZero(t *testing.T) {
	raw := []byte(`{"model":"qoder/r1","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`)
	resolver := ModelResolver(func(publicID string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, MaxInputTokens: 0}
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters is nil")
	}
	if c.Parameters.ContextLength != nil {
		t.Errorf("ContextLength = %v, want nil (zero not forwarded)", c.Parameters.ContextLength)
	}
}
