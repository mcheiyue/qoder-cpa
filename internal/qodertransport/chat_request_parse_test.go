package qodertransport

import (
	"testing"
)

// --- parseChatPayload tests ---

func TestParseChatPayload_Basic(t *testing.T) {
	raw := minimalJSON("")
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.Model != "gpt-4" {
		t.Errorf("Model: got %q, want %q", p.Model, "gpt-4")
	}
	if p.PublicModel != "qoder/gpt-4" {
		t.Errorf("PublicModel: got %q, want %q", p.PublicModel, "qoder/gpt-4")
	}
}

func TestParseChatPayload_EmptyModel(t *testing.T) {
	_, err := parseChatPayload([]byte(`{"model":"","messages":[{"role":"user","content":"hi"}]}`), nil)
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestParseChatPayload_NoMessages(t *testing.T) {
	_, err := parseChatPayload([]byte(`{"model":"gpt-4","messages":[]}`), nil)
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestParseChatPayload_InvalidJSON(t *testing.T) {
	_, err := parseChatPayload([]byte(`not json`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseChatPayload_WithResolver(t *testing.T) {
	raw := minimalJSON("")
	resolver := ModelResolver(func(publicID string) string {
		if publicID == "qoder/gpt-4" {
			return "internal-gpt4"
		}
		return ""
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.Model != "internal-gpt4" {
		t.Errorf("Model after resolve: got %q, want %q", p.Model, "internal-gpt4")
	}
}

func TestParseChatPayload_ResolverReturnsEmpty(t *testing.T) {
	raw := minimalJSON("")
	resolver := ModelResolver(func(publicID string) string { return "" })
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.Model != "gpt-4" {
		t.Errorf("Model: got %q, want %q", p.Model, "gpt-4")
	}
}

// --- Full payload parse: advanced fields ---

func TestParseChatPayload_ReasoningEffort(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"high"`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.ReasoningEffort == nil || *p.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort: got %v, want %q", p.ReasoningEffort, "high")
	}
}

func TestParseChatPayload_MaxCompletionTokens(t *testing.T) {
	raw := minimalJSON(`"max_completion_tokens":8192`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.MaxCompletionTokens == nil || *p.MaxCompletionTokens != 8192 {
		t.Errorf("MaxCompletionTokens: got %v, want 8192", p.MaxCompletionTokens)
	}
}

func TestParseChatPayload_ParallelToolCalls(t *testing.T) {
	raw := minimalJSON(`"parallel_tool_calls":false`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.ParallelToolCalls == nil || *p.ParallelToolCalls != false {
		t.Errorf("ParallelToolCalls: got %v, want false", p.ParallelToolCalls)
	}
}

func TestParseChatPayload_AllAdvancedFields(t *testing.T) {
	raw := minimalJSON(`"reasoning_effort":"low","max_completion_tokens":4096,"parallel_tool_calls":true,"max_tokens":2048`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.ReasoningEffort == nil || *p.ReasoningEffort != "low" {
		t.Errorf("ReasoningEffort: got %v, want %q", p.ReasoningEffort, "low")
	}
	if p.MaxCompletionTokens == nil || *p.MaxCompletionTokens != 4096 {
		t.Errorf("MaxCompletionTokens: got %v, want 4096", p.MaxCompletionTokens)
	}
	if p.ParallelToolCalls == nil || *p.ParallelToolCalls != true {
		t.Errorf("ParallelToolCalls: got %v, want true", p.ParallelToolCalls)
	}
	if p.MaxTokens == nil || *p.MaxTokens != 2048 {
		t.Errorf("MaxTokens: got %v, want 2048", p.MaxTokens)
	}
}

func TestParseChatPayload_AbsentAdvancedFieldsNil(t *testing.T) {
	raw := minimalJSON(`"max_tokens":1024,"temperature":0.7`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.ReasoningEffort != nil {
		t.Errorf("ReasoningEffort should be nil, got %v", *p.ReasoningEffort)
	}
	if p.MaxCompletionTokens != nil {
		t.Errorf("MaxCompletionTokens should be nil, got %v", *p.MaxCompletionTokens)
	}
	if p.ParallelToolCalls != nil {
		t.Errorf("ParallelToolCalls should be nil, got %v", *p.ParallelToolCalls)
	}
}

// --- public model prefix test ---

func TestParseChatPayload_AlreadyPrefixed(t *testing.T) {
	raw := []byte(`{"model":"qoder/gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.PublicModel != "qoder/gpt-4" {
		t.Errorf("PublicModel: got %q, want %q", p.PublicModel, "qoder/gpt-4")
	}
	if p.Model != "gpt-4" {
		t.Errorf("Model: got %q, want %q", p.Model, "gpt-4")
	}
}

func TestParseChatPayload_ModelWithResolverOverride(t *testing.T) {
	raw := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	resolver := ModelResolver(func(publicID string) string {
		return "upstream-v2/gpt-4-turbo"
	})
	p, err := parseChatPayload(raw, resolver)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	if p.Model != "upstream-v2/gpt-4-turbo" {
		t.Errorf("Model: got %q, want %q", p.Model, "upstream-v2/gpt-4-turbo")
	}
}
